//go:build integration

package app_test

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/ux"
)

// ringBoxes devolve a caixa (x, y, largura, altura) de cada contorno visível do overlay, pelo DOM do CDP.
func (h *harness) ringBoxes(t *testing.T) [][4]float64 {
	t.Helper()
	var out [][4]float64
	collect(h.overlayRoot(t), func(n *proto.DOMNode) {
		for i := 0; i+1 < len(n.Attributes); i += 2 {
			if n.Attributes[i] != "class" || !strings.HasPrefix(n.Attributes[i+1], "ring") {
				continue
			}
			box, err := proto.DOMGetBoxModel{BackendNodeID: n.BackendNodeID}.Call(h.page(t))
			if err != nil {
				continue // display: none não tem caixa
			}
			q := box.Model.Border
			out = append(out, [4]float64{q[0], q[1], q[2] - q[0], q[5] - q[1]})
		}
	})
	return out
}

// waitRing espera um contorno 6 px fora da caixa do elemento, nos quatro lados (o overlay acompanha
// o elemento a cada quadro).
func (h *harness) waitRing(t *testing.T, ref string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		el := boxOf(t, h.ok(t, "inspect", map[string]any{"ref": ref}))
		want := [4]float64{float64(el.Min.X - 6), float64(el.Min.Y - 6), float64(el.Dx() + 12), float64(el.Dy() + 12)}
		rings := h.ringBoxes(t)
		for _, r := range rings {
			if math.Abs(r[0]-want[0]) <= 1.5 && math.Abs(r[1]-want[1]) <= 1.5 && math.Abs(r[2]-want[2]) <= 1.5 && math.Abs(r[3]-want[3]) <= 1.5 {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no ring around %s at %v; rings: %v", ref, el, rings)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// ringStyle é o atributo style do contorno (para conferir os cantos).
func (h *harness) ringStyle(t *testing.T) string {
	t.Helper()
	var style string
	collect(h.overlayRoot(t), func(n *proto.DOMNode) {
		for i := 0; i+1 < len(n.Attributes); i += 2 {
			if n.Attributes[i] == "class" && strings.HasPrefix(n.Attributes[i+1], "ring") {
				for j := 0; j+1 < len(n.Attributes); j += 2 {
					if n.Attributes[j] == "style" {
						style = n.Attributes[j+1]
					}
				}
			}
		}
	})
	return style
}

// O destaque acompanha o elemento quando a página ou um contêiner rola (antes ficava parado onde o Go
// mediu), quando o elemento muda de tamanho, de lugar ou de cantos, e some quando ele sai da área
// visível do contêiner. A página tem scroll-behavior: smooth, por isso as rolagens aqui são instantâneas.
func TestHighlightFollowsScroll(t *testing.T) {
	ctx := context.Background()
	ov := h.app.Overlay
	h.ok(t, "open", map[string]any{"url": h.url("/pages/scroll-playground.html")})
	full := h.ok(t, "snapshot", map[string]any{"full": true})
	near := ref(t, full, "button", "Alvo perto")
	show := func(ref string) {
		b := boxOf(t, h.ok(t, "inspect", map[string]any{"ref": ref}))
		ov.ShowHighlight(ctx, ref, [4]int{b.Min.X, b.Min.Y, b.Dx(), b.Dy()}, time.Minute)
	}

	// Página: destaque de ação.
	show(near)
	h.waitRing(t, near)
	h.mainWorld(t, `() => { window.scrollTo({top: 120, behavior: 'instant'}); return {}; }`)
	h.waitRing(t, near)

	// O elemento muda de tamanho, de lugar e de forma (cantos redondos) com o destaque na tela.
	h.mainWorld(t, `() => { const b = [...document.querySelectorAll('button')].find((b) => b.textContent === 'Alvo perto');
		Object.assign(b.style, {padding: '24px 60px', marginLeft: '80px', borderRadius: '20px'}); return {}; }`)
	h.waitRing(t, near)
	if style := h.ringStyle(t); !strings.Contains(style, "border-radius: calc(26px)") {
		t.Fatalf("the ring should follow the element's rounded corners: %s", style)
	}

	// "Sua vez": o destaque que fica até a pessoa retomar, e é ela quem rola para digitar.
	b := boxOf(t, h.ok(t, "inspect", map[string]any{"ref": near}))
	ov.WaitForUser(ux.ReasonPassword, near, [4]int{b.Min.X, b.Min.Y, b.Dx(), b.Dy()})
	h.mainWorld(t, `() => { window.scrollTo({top: 60, behavior: 'instant'}); return {}; }`)
	h.waitRing(t, near)
	ov.Apply(ux.Event{Kind: ux.UserResume})

	// Contêiner com rolagem própria: acompanha e some quando o alvo sai da área visível dele. Antes, os
	// blocos "lazy" da página já carregam, para a altura não mudar no meio.
	h.mainWorld(t, `() => { document.querySelectorAll('[data-lazy]').forEach((e) => e.classList.add('loaded')); return {}; }`)
	inner := ref(t, full, "button", "Alvo no fim da lista")
	h.mainWorld(t, `() => { const n = document.querySelector('.nested'); n.scrollIntoView({block: 'center', behavior: 'instant'}); n.scrollTo({top: n.scrollHeight, behavior: 'instant'}); return {}; }`)
	show(inner)
	h.waitRing(t, inner)
	// Meio escondido pela borda de baixo do contêiner: o contorno envolve só a parte visível.
	clip := h.mainWorld(t, `() => { const n = document.querySelector('.nested'); n.scrollTo({top: n.scrollTop - 15, behavior: 'instant'});
		const r = n.getBoundingClientRect(); return {bottom: r.top + n.clientTop + n.clientHeight}; }`)["bottom"].(float64)
	top := float64(boxOf(t, h.ok(t, "inspect", map[string]any{"ref": inner})).Min.Y - 6)
	deadline := time.Now().Add(3 * time.Second)
	for {
		rings := h.ringBoxes(t)
		if len(rings) == 1 && math.Abs(rings[0][1]-top) <= 1.5 && math.Abs(rings[0][1]+rings[0][3]-(clip+6)) <= 1.5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the ring should wrap only the visible part (top %v, container bottom %v): %v", top, clip, rings)
		}
		time.Sleep(50 * time.Millisecond)
	}
	h.mainWorld(t, `() => { document.querySelector('.nested').scrollTo({top: 0, behavior: 'instant'}); return {}; }`)
	deadline = time.Now().Add(3 * time.Second)
	for len(h.ringBoxes(t)) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("the ring should hide when its element is scrolled out of the container: %v", h.ringBoxes(t))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
