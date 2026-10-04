//go:build integration

package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JonathanSantos/vitral/internal/cli"
	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/storagestate"
	"github.com/JonathanSantos/vitral/internal/ux"
)

// Sessões (pedido 4.2): login à mão num perfil, session_save, e outro Vitral com --session num perfil
// temporário já começa logado (o localStorage existe antes do primeiro script da página).
func TestSessionSaveAndRestore(t *testing.T) {
	dir := t.TempDir()
	var profileA string
	a, cleanupA := newHarness(func(c *config.Config) { c.SessionsDir, profileA = dir, c.ProfileDir })
	defer cleanupA()

	snap := a.ok(t, "open", map[string]any{"url": a.url("/pages/login.html")})
	mustContain(t, snap, `status "deslogado"`)
	a.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Entrar")})
	mustContain(t, a.ok(t, "snapshot", nil), `status "logado"`)

	out := a.ok(t, "session_save", map[string]any{"name": "portal"})
	mustContain(t, out, `saved session "portal": `, "localStorage of 1 origin(s): "+a.site.URL, "never commit or share it", "--session portal")
	for _, secret := range []string{"tk-7f3a91", "s-55c2", "portal_sess", "portal-token"} {
		if strings.Contains(out, secret) {
			t.Fatalf("session_save must never show the content (%s):\n%s", secret, out)
		}
	}
	a.fail(t, "session_save", map[string]any{"name": "../fora"}, "invalid session name")

	file := filepath.Join(dir, "portal.json")
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("session file %v %v", info, err)
	}
	st, err := storagestate.Store{Dir: dir}.Load("portal")
	if err != nil || st.Profile != "default" || len(st.Origins) != 1 || st.Origins[0].LocalStorage["portal-token"] != "tk-7f3a91" {
		t.Fatalf("%+v %v", st, err)
	}
	// Uma origem https (sem rede: a aba temporária intercepta a requisição) também é restaurada.
	st.Origins = append(st.Origins, storagestate.Origin{Origin: "https://portal.vitral.invalid", LocalStorage: map[string]string{"k": "v"}})
	st.Name = "portal-https"
	if _, err := (storagestate.Store{Dir: dir}).Save(st); err != nil {
		t.Fatal(err)
	}

	// Outro Vitral, com o perfil A ainda aberto: --session usa um perfil temporário.
	b, cleanupB := newHarness(func(c *config.Config) { c.SessionsDir, c.Session = dir, "portal-https" })
	defer cleanupB()
	start := time.Now()
	// A mesma origem do login (o localStorage é por origem, com a porta).
	mustContain(t, b.ok(t, "open", map[string]any{"url": a.url("/pages/login.html")}), `status "logado"`)
	t.Logf("open with --session (2 origins restored): %v", time.Since(start))
	// O selo mostra a sessão em uso.
	b.waitOverlayText(t, ux.TextIdle+" · sessão portal-https")
	// O estado restaurado (inclusive a origem https) pode ser salvo de novo nessa sessão.
	out = b.ok(t, "session_save", map[string]any{"name": "copia"})
	mustContain(t, out, "localStorage of 2 origin(s)", "https://portal.vitral.invalid")

	// vitral session save: com o perfil A aberto, explica o caminho pela conversa; fechado, salva sozinho
	// (as origens registradas no perfil, sem rede: o site de A nem precisa estar no ar).
	var out2 bytes.Buffer
	env := cli.Env{Stdout: &out2, Stderr: &out2}
	load := func(args []string) (config.Config, error) {
		cfg := config.Defaults(t.TempDir())
		cfg.ProfileDir, cfg.SessionsDir, cfg.BrowserPath = profileA, dir, testBrowserPath()
		return cfg, nil
	}
	if err := cli.SessionCommand(env, []string{"save", "cli"}, load); err == nil || !strings.Contains(err.Error(), "ferramenta session_save") {
		t.Fatalf("busy profile: %v", err)
	}
	a.app.Browser.Close()
	if err := cli.SessionCommand(env, []string{"save", "cli"}, load); err != nil {
		t.Fatal(err)
	}
	mustContain(t, out2.String(), "✓ sessão cli salva em "+filepath.Join(dir, "cli.json"), "localStorage de: "+a.site.URL, "nunca coloque no git")
	if strings.Contains(out2.String(), "tk-7f3a91") {
		t.Fatal("the CLI must not show the content")
	}
	out2.Reset()
	if err := cli.SessionCommand(env, []string{"list"}, load); err != nil {
		t.Fatal(err)
	}
	mustContain(t, out2.String(), "  cli: perfil default, salva em ", "  portal: ", "  portal-https: ", "https://portal.vitral.invalid")

	// Sessão que não existe: erro acionável na primeira ferramenta.
	c, cleanupC := newHarness(func(c *config.Config) { c.SessionsDir, c.Session = dir, "nao-existe" })
	defer cleanupC()
	c.fail(t, "open", map[string]any{"url": c.url("/pages/login.html")}, "session not found: nao-existe")
}
