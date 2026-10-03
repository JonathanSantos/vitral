//go:build integration

package app_test

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/goleak"

	"github.com/JonathanSantos/vitral/internal/app"
	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/test/fixtures"
	"github.com/JonathanSantos/vitral/test/procs"
)

// harness sobe um único Chromium headless para o pacote inteiro, falando via cliente MCP real.
type harness struct {
	site    *fixtures.Server
	session *mcp.ClientSession
	app     *app.App
}

var h *harness

// leaked marca vazamento de processo detectado em algum harness.
var leaked bool

func TestMain(m *testing.M) {
	flag.Parse()
	// O que já roda antes dos testes (runtime, flags) não conta como vazamento.
	baseline := goleak.IgnoreCurrent()
	var cleanup func()
	h, cleanup = newHarness(nil)
	code := m.Run()
	cleanup()
	if leaked {
		code = 1
	}
	// Goroutines: depois de fechar navegador e servidor, nada do Vitral pode continuar rodando.
	if err := goleak.Find(baseline); err != nil {
		fmt.Fprintln(os.Stderr, "goroutine leak:", err)
		code = 1
	}
	os.Exit(code)
}

// newHarness aceita mudanças de configuração (ex.: allowedHosts) para harnesses extras.
func newHarness(tweak func(*config.Config)) (*harness, func()) {
	site := fixtures.New()
	profile, err := os.MkdirTemp("", "vitral-test-*")
	if err != nil {
		panic(err)
	}
	cfg := config.Defaults(profile)
	cfg.Headless = true
	cfg.ProfileDir = profile
	cfg.StabilityTimeout = 5 * time.Second
	cfg.FlowsDir = filepath.Join(profile, "flows")
	// Testes usam um navegador explícito para o CI padrão não baixar o Chrome for Testing.
	cfg.BrowserPath = testBrowserPath()
	if tweak != nil {
		tweak(&cfg)
	}
	logOut := io.Discard
	level := slog.LevelInfo
	if os.Getenv("VITRAL_TEST_LOG") != "" {
		logOut, level = os.Stderr, slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: level}))
	a := app.Build(cfg, logger)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := a.Server.Connect(ctx, st, nil); err != nil {
		panic(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		panic(err)
	}
	return &harness{site: site, session: cs, app: a}, func() {
		pids := a.Browser.LaunchedPIDs()
		_ = cs.Close()
		a.Close()
		site.Close()
		// Vazamento de processos: o navegador lançado (e os filhos, pelo perfil) precisa ter morrido.
		if err := procs.WaitGone(15*time.Second, pids, profile); err != nil {
			fmt.Fprintln(os.Stderr, "process leak:", err)
			leaked = true
		}
		_ = os.RemoveAll(profile)
	}
}

// testBrowserPath escolhe o navegador dos testes: VITRAL_BROWSER_PATH, o Chrome for Testing
// gerenciado se já estiver baixado, ou o Chrome instalado na máquina.
func testBrowserPath() string {
	if p := os.Getenv(config.EnvBrowserPath); p != "" {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil {
		inst, _ := browser.NewInstaller(config.Defaults(home), nil)
		if _, err := os.Stat(inst.Executable()); err == nil {
			return inst.Executable()
		}
	}
	p, _ := launcher.LookPath()
	return p
}

func (h *harness) url(path string) string { return h.site.URL + path }

// call chama uma ferramenta e devolve o texto e se foi erro.
func (h *harness) call(t *testing.T, name string, args map[string]any) (string, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := h.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n"), res.IsError
}

// ok chama a ferramenta e falha o teste se ela devolver erro.
func (h *harness) ok(t *testing.T, name string, args map[string]any) string {
	t.Helper()
	out, isErr := h.call(t, name, args)
	if isErr {
		t.Fatalf("%s(%v) failed: %s", name, args, out)
	}
	return out
}

// fail chama a ferramenta e exige erro contendo want.
func (h *harness) fail(t *testing.T, name string, args map[string]any, want string) {
	t.Helper()
	out, isErr := h.call(t, name, args)
	if !isErr {
		t.Fatalf("%s(%v) should fail, got: %s", name, args, out)
	}
	if !strings.Contains(out, want) {
		t.Fatalf("%s error = %q, want it to contain %q", name, out, want)
	}
}

// ref acha a ref de um elemento pelo papel e nome no texto do snapshot.
func ref(t *testing.T, snapshot, role, name string) string {
	t.Helper()
	re := regexp.MustCompile(`\[(e\d+)\] ` + regexp.QuoteMeta(role) + ` "` + regexp.QuoteMeta(name) + `"`)
	m := re.FindStringSubmatch(snapshot)
	if m == nil {
		t.Fatalf("no %s %q in:\n%s", role, name, snapshot)
	}
	return m[1]
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func mustContain(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(text, w) {
			t.Fatalf("missing %q in:\n%s", w, text)
		}
	}
}

// golden compara com testdata/golden/<name>, trocando a porta aleatória do servidor.
func golden(t *testing.T, name, got string) {
	t.Helper()
	got = strings.ReplaceAll(got, h.site.URL, "http://fixture")
	// O número do passo gravado depende da ordem dos testes.
	got = regexp.MustCompile(`step #\d+`).ReplaceAllString(got, "step #N")
	path := filepath.Join("..", "..", "testdata", "golden", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if got != string(want) {
		t.Fatalf("snapshot differs from %s (run with -update to accept)\ngot:\n%s\nwant:\n%s", path, got, want)
	}
}
