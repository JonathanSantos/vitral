//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Segurança (docs/TESTING.md, seção 8), pelo binário real.
func TestSecurityAllowedHosts(t *testing.T) {
	s := start(t, "--allowed-hosts", "127.0.0.1")
	ctx := context.Background()

	other := strings.Replace(site.URL, "127.0.0.1", "localhost", 1)
	text, isErr, err := s.Call(ctx, "open", map[string]any{"url": other + "/pages/form.html"})
	if err != nil || !isErr || !strings.Contains(text, "host not allowed") {
		t.Fatalf("open outside the list: %v %v %s", err, isErr, text)
	}

	text, _, err = s.Call(ctx, "open", map[string]any{"url": site.URL + "/pages/external.html"})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := refIn([]string{text}, "link", "Site de fora")
	if err != nil {
		t.Fatal(err)
	}
	text, _, err = s.Call(ctx, "click", map[string]any{"ref": ref})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "is outside --allowed-hosts") {
		t.Fatalf("navigation by link should be blocked and reported:\n%s", text)
	}
	if strings.Contains(text, `textbox "Nome"`) {
		t.Fatalf("the blocked page must not load:\n%s", text)
	}
}

func TestSecurityMaskingAndPassword(t *testing.T) {
	s := start(t)
	ctx := context.Background()

	// Headers sensíveis: nunca na resposta do network_get nem no transcript salvo.
	text, _, _ := s.Call(ctx, "open", map[string]any{"url": site.URL + "/pages/api.html"})
	carregar, err := refIn([]string{text}, "button", "Carregar pedidos")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Call(ctx, "mock_add", map[string]any{"urlPattern": "/nada", "headers": map[string]any{"Authorization": "Bearer mock-secret"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Call(ctx, "click", map[string]any{"ref": carregar}); err != nil {
		t.Fatal(err)
	}
	list, _, _ := s.Call(ctx, "network_list", map[string]any{"filter": "/api/orders"})
	id := strings.Fields(list[strings.Index(list, "\n")+1:])[0]
	detail, _, _ := s.Call(ctx, "network_get", map[string]any{"id": atoi(t, id)})
	if strings.Contains(detail, "segredo-123") || !strings.Contains(detail, "Authorization: ***") {
		t.Fatalf("network_get leaked the Authorization header:\n%s", detail)
	}
	saved := filepath.Join(t.TempDir(), "transcript.json")
	s.writeTranscript(saved)
	data, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"segredo-123", "mock-secret"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("saved transcript leaks %q", secret)
		}
	}

	// Senha: o type nunca digita; o Vitral entrega a vez para a pessoa.
	form, _, _ := s.Call(ctx, "open", map[string]any{"url": site.URL + "/pages/form.html"})
	senha, err := refIn([]string{form}, "textbox", "Senha")
	if err != nil {
		t.Fatal(err)
	}
	reply, isErr, _ := s.Call(ctx, "type", map[string]any{"ref": senha, "text": "nao-digite"})
	if isErr || !strings.Contains(reply, "Waiting for the user") {
		t.Fatalf("password type should hand over to the user:\n%s", reply)
	}
	s.clickOverlay("toggle_pause")
	snap, _, _ := s.Call(ctx, "snapshot", map[string]any{})
	if !strings.Contains(snap, `textbox "Senha" type=password value=""`) {
		t.Fatalf("the password field must stay empty:\n%s", snap)
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			t.Fatalf("not a number: %q", s)
		}
		n = n*10 + int(r-'0')
	}
	return n
}
