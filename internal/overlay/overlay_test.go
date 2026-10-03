package overlay

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/ux"
	"github.com/JonathanSantos/vitral/internal/world"
)

func newTestManager() *Manager {
	return New(Options{}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.New())
}

// O payload da binding vem da página: só o token certo e os tipos permitidos mudam o estado.
func TestOnPageEventValidatesPayload(t *testing.T) {
	m := newTestManager()
	tests := []struct {
		name    string
		payload string
		blocked bool
	}{
		{"wrong token", `{"token":"chute","type":"toggle_pause"}`, false},
		{"missing token", `{"type":"toggle_pause"}`, false},
		{"malformed", `{toggle`, false},
		{"not allowed type", `{"token":"` + m.token + `","type":"pause"}`, false},
		{"wait_user is not for pages", `{"token":"` + m.token + `","type":"wait_user"}`, false},
		{"valid toggle", `{"token":"` + m.token + `","type":"toggle_pause"}`, true},
	}
	for _, tt := range tests {
		m.state = ux.State{Mode: ux.Idle}
		m.onPageEvent(tt.payload)
		if m.Blocked() != tt.blocked {
			t.Errorf("%s: blocked = %v, want %v", tt.name, m.Blocked(), tt.blocked)
		}
	}
}

func TestScriptAndView(t *testing.T) {
	script := Script("tok123", true)
	for _, want := range []string{`const TOKEN = "tok123"`, "const MINIMAL = true", "--vitral-ink:#1e1b2e;"} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q", want)
		}
	}
	if strings.Contains(script, "__TOKEN__") || strings.Contains(script, "__CSS_VARS__") {
		t.Fatal("placeholders left in the script")
	}
	if a, b := newToken(), newToken(); a == b || len(a) != 32 {
		t.Fatal("token must be random")
	}
	v := ViewOf(ux.State{Mode: ux.Waiting, Reason: "x", Hold: &ux.Highlight{Ref: "e5", Box: ux.Box{1, 2, 3, 4}}})
	if !v.Blocked || v.Button != ux.TextResume || v.Hold == nil || v.Hold.Mark != 5 || v.Hold.Text != "white" {
		t.Fatalf("view = %+v hold=%+v", v, v.Hold)
	}
}
