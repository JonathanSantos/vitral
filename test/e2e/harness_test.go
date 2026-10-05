//go:build e2e

// Package e2e testa o Vitral como o Copilot usa: o binário real, falando MCP por stdio.
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/test/fakeagent"
	"github.com/JonathanSantos/vitral/test/fixtures"
	"github.com/JonathanSantos/vitral/test/procs"
	"github.com/JonathanSantos/vitral/test/tokens"
)

var (
	binPath     string           // vitral compilado com -cover
	browsersDir string           // Chrome for Testing de teste (cache entre execuções)
	browserArgs []string         // flags de navegador comuns a todos os cenários
	site        *fixtures.Server // páginas de fixture
	coverDir    string           // GOCOVERDIR do binário
	outDir      string           // artefatos de falha
)

func TestMain(m *testing.M) {
	if err := setup(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e setup:", err)
		os.Exit(1)
	}
	code := m.Run()
	site.Close()
	if err := writeBudget(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: write token report:", err)
		code = 1
	}
	os.Exit(code)
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func setup() error {
	root := repoRoot()
	outDir = envOr("VITRAL_E2E_OUT", filepath.Join(root, ".quality", "out", "e2e"))
	coverDir = envOr("VITRAL_E2E_COVERDIR", filepath.Join(outDir, "cover"))
	for _, d := range []string{outDir, coverDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return err
		}
	}
	binDir, err := os.MkdirTemp("", "vitral-e2e-bin-*")
	if err != nil {
		return err
	}
	binPath = filepath.Join(binDir, "vitral")
	// -race: uma data race no servidor durante os cenários (inclusive o caótico) reprova o teste.
	build := exec.Command("go", "build", "-race", "-cover", "-coverpkg=./internal/...,./cmd/vitral", "-o", binPath, "./cmd/vitral")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build: %v\n%s", err, out)
	}

	// Chrome for Testing gerenciado num diretório de teste; sem rede, aceita um navegador explícito.
	cache, _ := os.UserCacheDir()
	browsersDir = envOr("VITRAL_E2E_BROWSERS_DIR", filepath.Join(cache, "vitral-e2e", "browsers"))
	install := exec.Command(binPath, "install-browser", "--browsers-dir", browsersDir)
	install.Env = append(os.Environ(), "GOCOVERDIR="+coverDir)
	if out, err := install.CombinedOutput(); err != nil {
		explicit := os.Getenv("VITRAL_BROWSER_PATH")
		if explicit == "" {
			return fmt.Errorf("install-browser failed (set VITRAL_BROWSER_PATH to run offline): %v\n%s", err, out)
		}
		browserArgs = []string{"--browser-path", explicit}
	}
	browserArgs = append(browserArgs, "--browsers-dir", browsersDir)
	site = fixtures.New()
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Session é um Vitral rodando por stdio, com transcript e validação do stdout.
type Session struct {
	t          *testing.T
	cs         *mcp.ClientSession
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	home       string
	debugInfo  string
	stdout     *jsonLineChecker
	stderr     *bytes.Buffer
	closeOnce  sync.Once
	mu         sync.Mutex
	transcript []fakeagent.Exchange
	lastText   string
	tokens     int
}

// start sobe um Vitral isolado (HOME, perfil e fluxos temporários) e conecta o cliente MCP.
func start(t *testing.T, extraArgs ...string) *Session {
	t.Helper()
	return startBin(t, binPath, extraArgs...)
}

// startBin é o start com outro binário (o de medição de tempo, sem -race).
func startBin(t *testing.T, bin string, extraArgs ...string) *Session {
	t.Helper()
	home := t.TempDir()
	s := &Session{t: t, home: home, debugInfo: filepath.Join(home, "debug.json"), stderr: &bytes.Buffer{}}
	args := append([]string{
		"--headless", "--profile-dir", filepath.Join(home, "profile"), "--flows-dir", filepath.Join(home, "flows"),
		"--lang", "pt-BR", "--window-size", "1280,800", "--debug-info", s.debugInfo, "--password-store", "mock",
	}, browserArgs...)
	args = append(args, extraArgs...)
	// make demo-cursor: janela visível para uma pessoa acompanhar (e gravar o GIF).
	if os.Getenv("VITRAL_E2E_VISIBLE") != "" {
		args = args[1:]
	}
	s.cmd = exec.Command(bin, args...)
	s.cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "TZ=UTC", "GOCOVERDIR="+coverDir,
		"PUPPETEER_CACHE_DIR="+filepath.Join(home, "none"), "PLAYWRIGHT_BROWSERS_PATH="+filepath.Join(home, "none"))
	s.cmd.Stderr = s.stderr
	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	s.stdin = stdin
	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s.stdout = newJSONLineChecker(stdout)
	client := mcp.NewClient(&mcp.Implementation{Name: "vitral-e2e"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.cs, err = client.Connect(ctx, &mcp.IOTransport{Reader: s.stdout, Writer: stdin}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	return s
}

// close salva artefatos em falha, fecha o stdin e exige que o processo termine sozinho e leve
// junto o navegador que lançou (docs/TESTING.md, seção 9). Pode ser chamado mais de uma vez.
func (s *Session) close() {
	s.closeOnce.Do(s.doClose)
}

func (s *Session) doClose() {
	if s.t.Failed() {
		s.saveArtifacts()
	}
	// Como o VS Code ao parar o servidor: fecha o stdin primeiro (o Close do cliente esperaria
	// as chamadas pendentes terminarem).
	_ = s.stdin.Close()
	_ = s.cs.Close()
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		_ = s.cmd.Process.Kill()
		s.t.Errorf("vitral did not exit after stdin was closed\nstderr:\n%s", s.stderr.String())
	}
	info := s.readDebugInfo()
	if !info.Attached {
		if err := procs.WaitGone(15*time.Second, []int{info.PID}, filepath.Join(s.home, "profile")); err != nil {
			s.t.Errorf("browser leaked after the server exited: %v", err)
		}
	}
	if strings.Contains(s.stderr.String(), "DATA RACE") {
		s.t.Errorf("data race in the server:\n%s", s.stderr.String())
	}
	if bad := s.stdout.problems(); len(bad) > 0 {
		s.t.Errorf("stdout had bytes outside the MCP protocol: %q", bad)
	}
}

func (s *Session) saveArtifacts() {
	dir := filepath.Join(outDir, sanitize(s.t.Name()))
	_ = os.MkdirAll(dir, 0o750)
	s.writeTranscript(filepath.Join(dir, "transcript.json"))
	_ = os.WriteFile(filepath.Join(dir, "stderr.log"), s.stderr.Bytes(), 0o600)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if res, err := s.cs.CallTool(ctx, &mcp.CallToolParams{Name: "snapshot", Arguments: map[string]any{}}); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "last-snapshot.txt"), []byte(textOf(res)), 0o600)
	}
	if res, err := s.cs.CallTool(ctx, &mcp.CallToolParams{Name: "screenshot", Arguments: map[string]any{}}); err == nil {
		for _, c := range res.Content {
			if img, ok := c.(*mcp.ImageContent); ok {
				_ = os.WriteFile(filepath.Join(dir, "screenshot.png"), img.Data, 0o600)
			}
		}
	}
	s.t.Logf("artifacts saved in %s", dir)
}

// writeTranscript salva o transcript com os headers sensíveis mascarados.
func (s *Session) writeTranscript(path string) {
	s.mu.Lock()
	masked := fakeagent.MaskTranscript(s.transcript)
	s.mu.Unlock()
	data, _ := json.MarshalIndent(masked, "", "  ")
	_ = os.WriteFile(path, data, 0o600)
}

func sanitize(name string) string {
	return strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(name)
}

func textOf(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// Call implementa fakeagent.Caller e registra tudo no transcript.
func (s *Session) Call(ctx context.Context, tool string, args map[string]any) (string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if args == nil {
		args = map[string]any{}
	}
	res, err := s.cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return "", false, err
	}
	text := textOf(res)
	s.mu.Lock()
	s.transcript = append(s.transcript, fakeagent.Exchange{Tool: tool, Args: args, Text: text, IsError: res.IsError})
	s.lastText = text
	s.tokens += tokens.Estimate(text)
	s.mu.Unlock()
	return text, res.IsError, nil
}

// debugInfo é o que o Vitral grava com --debug-info (URL do CDP, PID do navegador lançado).
type debugInfo struct {
	ControlURL string `json:"controlURL"`
	PID        int    `json:"pid"`
	Attached   bool   `json:"attached"`
}

func (s *Session) readDebugInfo() debugInfo {
	var info debugInfo
	if data, err := os.ReadFile(s.debugInfo); err == nil {
		_ = json.Unmarshal(data, &info)
	}
	return info
}

// history devolve os textos das respostas, do mais antigo ao mais recente.
func (s *Session) history() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.transcript))
	for i, e := range s.transcript {
		out[i] = e.Text
	}
	return out
}

// browser conecta um segundo cliente CDP no navegador do Vitral (para clicar no selo como a pessoa).
// Para soltar, chame o cancel devolvido: rod.Browser.Close() mandaria Browser.close e fecharia o
// navegador do Vitral.
func (s *Session) browser() (context.CancelFunc, *rod.Page) {
	s.t.Helper()
	data, err := os.ReadFile(s.debugInfo)
	if err != nil {
		s.t.Fatalf("debug info not written (did any tool open the browser?): %v", err)
	}
	var info struct {
		ControlURL string `json:"controlURL"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		s.t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := rod.New().Context(ctx).ControlURL(info.ControlURL)
	if err := b.Connect(); err != nil {
		cancel()
		s.t.Fatal(err)
	}
	pages, err := b.Pages()
	if err != nil || len(pages) == 0 {
		cancel()
		s.t.Fatalf("no page: %v", err)
	}
	return cancel, pages[0]
}

// overlayButton acha um botão do selo pelo data-action, atravessando o shadow root fechado.
func overlayNodes(p *rod.Page) (*proto.DOMNode, error) {
	depth := -1
	doc, err := proto.DOMGetDocument{Depth: &depth, Pierce: true}.Call(p)
	if err != nil {
		return nil, err
	}
	var found *proto.DOMNode
	var walk func(n *proto.DOMNode)
	walk = func(n *proto.DOMNode) {
		if n == nil || found != nil {
			return
		}
		if n.NodeName == "VITRAL-OVERLAY" && len(n.ShadowRoots) > 0 {
			found = n.ShadowRoots[0]
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc.Root)
	if found == nil {
		return nil, fmt.Errorf("overlay not found")
	}
	return found, nil
}

func each(n *proto.DOMNode, fn func(*proto.DOMNode)) {
	if n == nil {
		return
	}
	fn(n)
	for _, c := range n.Children {
		each(c, fn)
	}
}

// clickOverlay clica no botão do selo com o mouse, pelo CDP (simula a pessoa).
func (s *Session) clickOverlay(action string) {
	s.t.Helper()
	disconnect, p := s.browser()
	defer disconnect()
	eventually(s.t, 10*time.Second, func() error {
		root, err := overlayNodes(p)
		if err != nil {
			return err
		}
		var target *proto.DOMNode
		each(root, func(n *proto.DOMNode) {
			for i := 0; i+1 < len(n.Attributes); i += 2 {
				if n.Attributes[i] == "data-action" && n.Attributes[i+1] == action {
					target = n
				}
			}
		})
		if target == nil {
			return fmt.Errorf("button %q not found", action)
		}
		box, err := proto.DOMGetBoxModel{BackendNodeID: target.BackendNodeID}.Call(p)
		if err != nil {
			return err
		}
		q := box.Model.Content
		x, y := (q[0]+q[2]+q[4]+q[6])/4, (q[1]+q[3]+q[5]+q[7])/4
		if err := p.Mouse.MoveTo(proto.Point{X: x, Y: y}); err != nil {
			return err
		}
		return p.Mouse.Click(proto.InputMouseButtonLeft, 1)
	})
}

// overlayText devolve o texto visível do selo.
func (s *Session) overlayText() string {
	disconnect, p := s.browser()
	defer disconnect()
	root, err := overlayNodes(p)
	if err != nil {
		return ""
	}
	var parts []string
	each(root, func(n *proto.DOMNode) {
		if n.NodeType == 3 && strings.TrimSpace(n.NodeValue) != "" && !strings.Contains(n.NodeValue, "{") {
			parts = append(parts, strings.TrimSpace(n.NodeValue))
		}
	})
	return strings.Join(parts, " | ")
}

// mainWorld roda JS no mundo da página (o harness mudando a página "por fora" do agente).
func (s *Session) mainWorld(js string) {
	s.t.Helper()
	disconnect, p := s.browser()
	defer disconnect()
	if _, err := p.Eval(js); err != nil {
		s.t.Fatal(err)
	}
}

// eventually repete a condição até ela passar ou o tempo acabar (espera por condição, não por sleep).
func eventually(t *testing.T, timeout time.Duration, cond func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		err := cond()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met in %s: %v", timeout, err)
		}
		<-tick.C
	}
}

// jsonLineChecker repassa o stdout para o cliente MCP e confere que toda linha é JSON-RPC 2.0.
type jsonLineChecker struct {
	r       io.Reader
	pr      *io.PipeReader
	mu      sync.Mutex
	invalid []string
}

func newJSONLineChecker(r io.Reader) *jsonLineChecker {
	pr, pw := io.Pipe()
	c := &jsonLineChecker{r: r, pr: pr}
	go func() {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
		for sc.Scan() {
			line := sc.Bytes()
			var msg struct {
				JSONRPC string `json:"jsonrpc"`
			}
			if err := json.Unmarshal(line, &msg); err != nil || msg.JSONRPC != "2.0" {
				c.mu.Lock()
				c.invalid = append(c.invalid, string(line))
				c.mu.Unlock()
			}
			if _, err := pw.Write(append(append([]byte{}, line...), '\n')); err != nil {
				return
			}
		}
		_ = pw.CloseWithError(sc.Err())
	}()
	return c
}

func (c *jsonLineChecker) Read(p []byte) (int, error) { return c.pr.Read(p) }

func (c *jsonLineChecker) Close() error { return c.pr.Close() }

func (c *jsonLineChecker) problems() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.invalid
}
