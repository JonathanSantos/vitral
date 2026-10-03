//go:build integration

package app_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (h *harness) page(t *testing.T) *rod.Page {
	t.Helper()
	p := h.app.Browser.Current()
	if p == nil {
		t.Fatal("no active page")
	}
	return p
}

// overlayRoot acha o shadow root (fechado) do <vitral-overlay> pelo DOM do CDP, como faria o DevTools.
func (h *harness) overlayRoot(t *testing.T) *proto.DOMNode {
	t.Helper()
	depth := -1
	doc, err := proto.DOMGetDocument{Depth: &depth, Pierce: true}.Call(h.page(t))
	if err != nil {
		t.Fatal(err)
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
	return found
}

func collect(n *proto.DOMNode, fn func(*proto.DOMNode)) {
	if n == nil {
		return
	}
	fn(n)
	for _, c := range n.Children {
		collect(c, fn)
	}
}

// overlayText devolve o texto visível do selo ("" se o overlay não existe).
func (h *harness) overlayText(t *testing.T) string {
	t.Helper()
	var parts []string
	collect(h.overlayRoot(t), func(n *proto.DOMNode) {
		if n.NodeType == 3 && n.ParentID != 0 && strings.TrimSpace(n.NodeValue) != "" && !strings.Contains(n.NodeValue, "{") {
			parts = append(parts, strings.TrimSpace(n.NodeValue))
		}
	})
	return strings.Join(parts, " | ")
}

// waitOverlayText espera o selo mostrar um texto (o render é assíncrono).
func (h *harness) waitOverlayText(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := h.overlayText(t)
		if strings.Contains(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("overlay text %q does not contain %q", got, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// clickOverlay clica com o mouse de verdade num botão do selo (data-action).
// Tenta de novo se o selo for redesenhado entre achar o botão e medir a caixa.
func (h *harness) clickOverlay(t *testing.T, action string) {
	t.Helper()
	var err error
	for range 10 {
		if err = h.tryClickOverlay(t, action); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("click overlay %q: %v", action, err)
}

func (h *harness) tryClickOverlay(t *testing.T, action string) error {
	t.Helper()
	var target *proto.DOMNode
	collect(h.overlayRoot(t), func(n *proto.DOMNode) {
		for i := 0; i+1 < len(n.Attributes); i += 2 {
			if n.Attributes[i] == "data-action" && n.Attributes[i+1] == action {
				target = n
			}
		}
	})
	if target == nil {
		return fmt.Errorf("button not found (overlay: %q)", h.overlayText(t))
	}
	p := h.page(t)
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
}

// resume retoma pelo botão do selo, como a pessoa faria, se o Vitral estiver pausado.
func (h *harness) resume(t *testing.T) {
	t.Helper()
	if !h.app.Overlay.Blocked() {
		return
	}
	h.clickOverlay(t, "toggle_pause")
	deadline := time.Now().Add(5 * time.Second)
	for h.app.Overlay.Blocked() {
		if time.Now().After(deadline) {
			t.Fatal("resume did not unblock")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
