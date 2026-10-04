package overlay

import (
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/ux"
)

// FuzzOnPageEvent: o payload vem da página (não confiável). Sem o token certo, nada muda.
func FuzzOnPageEvent(f *testing.F) {
	f.Add(`{"token":"x","type":"toggle_pause"}`)
	f.Add(`{"type":"toggle_pause"}`)
	f.Add(`{"token":null}`)
	f.Add(`{toggle`)
	f.Add(`{"token":"","type":"collapse"}`)
	f.Add(`{"token":"x","type":"cursor_done","id":"1"}`)
	f.Fuzz(func(t *testing.T, payload string) {
		m := newTestManager()
		if strings.Contains(payload, m.token) {
			t.Skip()
		}
		m.state = ux.State{Mode: ux.Idle}
		ch := make(chan struct{}, 1)
		m.waiters = map[string]chan struct{}{"1": ch}
		m.onPageEvent(payload)
		if m.state != (ux.State{Mode: ux.Idle}) {
			t.Fatalf("payload without token changed the state: %+v", m.state)
		}
		if len(ch) != 0 {
			t.Fatal("payload without token finished a cursor plan")
		}
	})
}
