//go:build retouchdiag

package browser_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
)

// Medições dos Retoques (docs/RETOQUES.md, seção 9), antes de qualquer código da feature:
// go test -tags retouchdiag -run TestRetouchDiag -v ./internal/browser/
// O navegador é o Chrome for Testing fixado do cache do e2e (ou VITRAL_BROWSER_PATH).

func diagBrowser(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("VITRAL_BROWSER_PATH"); p != "" {
		return p
	}
	cache, _ := os.UserCacheDir()
	m, _ := filepath.Glob(filepath.Join(cache, "vitral-e2e", "browsers", "*", "chrome-*", "*.app", "Contents", "MacOS", "*"))
	if len(m) == 0 {
		t.Skip("no Chrome for Testing in the e2e cache (run make e2e once)")
	}
	return m[0]
}

// Página que registra, no primeiro script dela (no head, durante o parse), o que os content scripts
// já tinham feito: a variável do CSS, o atributo do mundo isolado e a marca do mundo da página.
const diagPage = `<!doctype html><html><head><title>diag</title><script>
window.__atParse = {
  css: getComputedStyle(document.documentElement).getPropertyValue('--retoque').trim(),
  iso: document.documentElement.getAttribute('data-iso'),
  main: window.__retoqueMain || null,
};
</script></head><body><h1>Diag</h1></body></html>`

// writeExt cria uma extensão MV3 com n retoques: cada um é um content script em document_start que
// põe um <style> (o CSS do Retoque) e marca o mundo isolado; o primeiro também tem uma versão no
// mundo da página.
func writeExt(t *testing.T, dir string, n int, cssValue string) {
	t.Helper()
	var scripts []string
	for i := range n {
		js := fmt.Sprintf(`(() => { const s = document.createElement('style'); s.dataset.retoque = '%d';
  s.textContent = ':root { --retoque: %s; } .r%d { color: red; }';
  (document.head || document.documentElement).append(s);
  document.documentElement.setAttribute('data-iso', '%s'); })();`, i, cssValue, i, cssValue)
		name := fmt.Sprintf("r%d.js", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(js), 0o644); err != nil {
			t.Fatal(err)
		}
		scripts = append(scripts, fmt.Sprintf(`{"matches": ["<all_urls>"], "js": [%q], "run_at": "document_start"}`, name))
	}
	if err := os.WriteFile(filepath.Join(dir, "main.js"), []byte(`window.__retoqueMain = 'page-world';`), 0o644); err != nil {
		t.Fatal(err)
	}
	scripts = append(scripts, `{"matches": ["<all_urls>"], "js": ["main.js"], "run_at": "document_start", "world": "MAIN"}`)
	manifest := fmt.Sprintf(`{"manifest_version": 3, "name": "Vitral Retoques (diag)", "version": "1.0",
  "background": {"service_worker": "sw.js"}, "action": {"default_title": "Vitral", "default_popup": "popup.html"},
  "content_scripts": [%s]}`, strings.Join(scripts, ","))
	for name, body := range map[string]string{
		"manifest.json": manifest,
		"sw.js":         `self.addEventListener('install', () => {});`,
		"popup.html":    `<!doctype html><p>Retoques</p>`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// launchDiag sobe o navegador com o perfil e as extensões dadas. automation=false tira as bandeiras de
// automação que o rod põe por padrão (o jeito do vitral open, seção 6.2).
func launchDiag(t *testing.T, bin, profile string, automation bool, exts ...string) (*rod.Browser, *launcher.Launcher, time.Duration) {
	t.Helper()
	l := launcher.New().Bin(bin).Headless(!visible).UserDataDir(profile).Set("remote-debugging-address", "127.0.0.1")
	if visible { // janela de verdade, fora da tela, para medir o que o modo sem janela muda
		l = l.Set("window-position", "-2400", "-2400")
	}
	if len(exts) > 0 {
		l = l.Set("load-extension", strings.Join(exts, ","))
	}
	if !automation {
		for _, f := range []string{"enable-automation", "disable-site-isolation-trials", "disable-popup-blocking",
			"disable-client-side-phishing-detection", "disable-background-networking", "disable-default-apps",
			"disable-component-extensions-with-background-pages", "disable-sync", "metrics-recording-only"} {
			l = l.Delete(flags.Flag(f))
		}
		l = l.Delete("disable-features").Delete("enable-features")
	}
	start := time.Now()
	u, err := l.Launch()
	if err != nil {
		t.Fatal(err)
	}
	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		t.Fatal(err)
	}
	return b, l, time.Since(start)
}

// waitSW espera o service worker da extensão aparecer entre os alvos.
func waitSW(t *testing.T, b *rod.Browser) (*proto.TargetTargetInfo, time.Duration) {
	t.Helper()
	start := time.Now()
	for time.Since(start) < 10*time.Second {
		res, err := proto.TargetGetTargets{}.Call(b)
		if err == nil {
			for _, ti := range res.TargetInfos {
				if ti.Type == "service_worker" && strings.HasPrefix(ti.URL, "chrome-extension://") {
					return ti, time.Since(start)
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, time.Since(start)
}

func atParse(t *testing.T, b *rod.Browser, url string) map[string]any {
	t.Helper()
	p, err := b.Page(proto.TargetCreateTarget{URL: url})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.WaitLoad(); err != nil {
		t.Fatal(err)
	}
	res, err := p.Eval(`() => ({...window.__atParse, webdriver: navigator.webdriver,
		styles: document.querySelectorAll('style[data-retoque]').length})`)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for k, v := range res.Value.Map() {
		out[k] = v.Val()
	}
	return out
}

// visible mede com janela de verdade (fora da tela): VITRAL_DIAG_VISIBLE=1.
var visible = os.Getenv("VITRAL_DIAG_VISIBLE") != ""

// Só o que depende da janela: navigator.webdriver no lançamento do vitral open (com e sem a porta de
// depuração) e o segundo lançamento no mesmo perfil (o Chrome com janela entrega para o que já roda).
func TestRetouchDiagVisible(t *testing.T) {
	if !visible {
		t.Skip("set VITRAL_DIAG_VISIBLE=1")
	}
	bin := diagBrowser(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(diagPage))
	}))
	defer site.Close()
	for _, automation := range []bool{true, false} {
		profile := t.TempDir()
		b, l, _ := launchDiag(t, bin, profile, automation)
		t.Logf("V automation=%v: at parse %v", automation, atParse(t, b, site.URL))
		if !automation {
			start := time.Now()
			second := launcher.New().Bin(bin).Headless(false).UserDataDir(profile).Set("window-position", "-2400", "-2400")
			_, err := second.Launch()
			t.Logf("V second launch on the same profile: err=%v after %v", err, time.Since(start))
			second.Kill()
			res, _ := proto.TargetGetTargets{}.Call(b)
			var pages []string
			for _, ti := range res.TargetInfos {
				if ti.Type == "page" {
					pages = append(pages, ti.URL)
				}
			}
			t.Logf("V pages in the first browser after the second launch: %v", pages)
		}
		b.MustClose()
		l.Cleanup()
	}
}

func TestRetouchDiag(t *testing.T) {
	bin := diagBrowser(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(diagPage))
	}))
	defer site.Close()

	// 9.1 --load-extension, document_start, mundo da página, service worker; com e sem automação.
	for _, automation := range []bool{true, false} {
		ext := t.TempDir()
		writeExt(t, ext, 1, "1")
		b, l, launchT := launchDiag(t, bin, t.TempDir(), automation, ext)
		sw, swT := waitSW(t, b)
		t.Logf("9.1 automation=%v: launch %v, service worker %v after %v", automation, launchT, sw != nil, swT)
		t.Logf("9.1 automation=%v: at parse %v", automation, atParse(t, b, site.URL))
		b.MustClose()
		l.Cleanup()
	}

	// 9.2 Recarregar sem reiniciar: (a) chrome.runtime.reload() pelo service worker; (b) Extensions.loadUnpacked.
	ext := t.TempDir()
	writeExt(t, ext, 1, "1")
	b, l, _ := launchDiag(t, bin, t.TempDir(), true, ext)
	sw, _ := waitSW(t, b)
	if sw == nil {
		t.Fatal("no service worker")
	}
	writeExt(t, ext, 1, "2") // o retoque mudou no disco
	start := time.Now()
	sess, err := proto.TargetAttachToTarget{TargetID: sw.TargetID, Flatten: true}.Call(b)
	if err != nil {
		t.Fatal(err)
	}
	swb := b.WithPanic(nil)
	_, err = proto.RuntimeEvaluate{Expression: `chrome.runtime.reload()`}.Call(&sessionCaller{b: swb, id: sess.SessionID})
	t.Logf("9.2a runtime.reload via service worker: err=%v", err)
	var sw2 *proto.TargetTargetInfo
	for time.Since(start) < 10*time.Second {
		if cur, _ := waitSW(t, b); cur != nil && cur.TargetID != sw.TargetID {
			sw2 = cur
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Logf("9.2a new service worker %v after %v; at parse after reload %v", sw2 != nil, time.Since(start), atParse(t, b, site.URL))
	other := t.TempDir()
	writeExt(t, other, 1, "3")
	res, err := proto.ExtensionsLoadUnpacked{Path: other}.Call(b)
	t.Logf("9.2b Extensions.loadUnpacked over websocket: %+v err=%v; at parse after load %v", res, err, atParse(t, b, site.URL))
	b.MustClose()
	l.Cleanup()

	// 9.4 Attach a um navegador "vitral open" (sem automação): DevToolsActivePort, aba nova da IA com
	// setAutomationOverride, e a aba da pessoa intocada; depois, segundo lançamento no mesmo perfil.
	profile := t.TempDir()
	b, l, _ = launchDiag(t, bin, profile, false)
	data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	t.Logf("9.4 DevToolsActivePort: %q err=%v", strings.ReplaceAll(string(data), "\n", " | "), err)
	person, err := b.Page(proto.TargetCreateTarget{URL: site.URL})
	if err != nil {
		t.Fatal(err)
	}
	_ = person.WaitLoad()
	ai, err := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		t.Fatal(err)
	}
	err = proto.EmulationSetAutomationOverride{Enabled: true}.Call(ai)
	_ = ai.Navigate(site.URL)
	_ = ai.WaitLoad()
	wdAI, _ := ai.Eval(`() => navigator.webdriver`)
	wdPerson, _ := person.Eval(`() => navigator.webdriver`)
	t.Logf("9.4 setAutomationOverride err=%v; webdriver: AI tab %v, person tab %v", err, wdAI.Value, wdPerson.Value)
	start = time.Now()
	second := launcher.New().Bin(bin).Headless(true).UserDataDir(profile)
	_, err = second.Launch()
	t.Logf("9.4 second launch on the same profile: %v after %v", err, time.Since(start))
	second.Cleanup()
	b.MustClose()
	l.Cleanup()

	// 9.5 Custo: lançamento até o service worker, e o CSS no primeiro script da página, com 0, 10 e 50 retoques.
	for _, n := range []int{0, 10, 50} {
		ext := t.TempDir()
		writeExt(t, ext, n, "1")
		var times []time.Duration
		var parse map[string]any
		for range 3 {
			b, l, launchT := launchDiag(t, bin, t.TempDir(), false, ext)
			_, swT := waitSW(t, b)
			times = append(times, launchT+swT)
			parse = atParse(t, b, site.URL)
			b.MustClose()
			l.Cleanup()
		}
		t.Logf("9.5 %d retouches: launch+sw %v; at parse %v", n, times, parse)
	}
}

// sessionCaller manda um comando CDP para uma sessão (o service worker) pela conexão do navegador.
type sessionCaller struct {
	b  *rod.Browser
	id proto.TargetSessionID
}

func (s *sessionCaller) GetSessionID() proto.TargetSessionID { return s.id }

func (s *sessionCaller) Call(ctx context.Context, sessionID, method string, params any) ([]byte, error) {
	return s.b.Call(ctx, string(s.id), method, params)
}

// O que liga navigator.webdriver: a página reporta o valor por fetch, sem CDP na aba. Chrome lançado à
// mão com o mínimo de bandeiras, mais uma de cada vez. Também: loadUnpacked de novo no mesmo caminho
// recarrega a extensão?
func TestRetouchDiagWebdriver(t *testing.T) {
	if !visible {
		t.Skip("set VITRAL_DIAG_VISIBLE=1")
	}
	bin := diagBrowser(t)
	got := make(chan string, 4)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/report" {
			got <- r.URL.RawQuery
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<script>fetch('/report?wd=' + navigator.webdriver)</script>`))
	}))
	defer site.Close()
	for _, extra := range [][]string{nil, {"--remote-debugging-port=0"}, {"--remote-debugging-pipe"}, {"--enable-automation"}} {
		args := append([]string{"--user-data-dir=" + t.TempDir(), "--no-first-run", "--window-position=-2400,-2400"}, extra...)
		cmd := exec.Command(bin, append(args, site.URL)...)
		if len(extra) == 1 && extra[0] == "--remote-debugging-pipe" {
			// O Chrome lê comandos no fd 3 e escreve no fd 4: os dois ficam abertos, sem mandar nada.
			inR, inW, _ := os.Pipe()
			outR, outW, _ := os.Pipe()
			defer inW.Close()
			defer outR.Close()
			cmd.ExtraFiles = []*os.File{inR, outW}
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		select {
		case v := <-got:
			t.Logf("W flags %v: %s", extra, v)
		case <-time.After(15 * time.Second):
			t.Logf("W flags %v: no report", extra)
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

// loadUnpacked no mesmo caminho depois de mudar os arquivos.
func TestRetouchDiagReloadSamePath(t *testing.T) {
	bin := diagBrowser(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(diagPage))
	}))
	defer site.Close()
	b, l, _ := launchDiag(t, bin, t.TempDir(), true)
	defer l.Cleanup()
	defer b.MustClose()
	ext := t.TempDir()
	writeExt(t, ext, 1, "1")
	r1, err := proto.ExtensionsLoadUnpacked{Path: ext}.Call(b)
	t.Logf("R first load: %+v err=%v; %v", r1, err, atParse(t, b, site.URL))
	writeExt(t, ext, 1, "2")
	start := time.Now()
	r2, err := proto.ExtensionsLoadUnpacked{Path: ext}.Call(b)
	t.Logf("R same path again after %v: %+v err=%v; %v", time.Since(start), r2, err, atParse(t, b, site.URL))
}
