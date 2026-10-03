//go:build integration

package app_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// shot chama screenshot e devolve a imagem decodificada e a legenda.
func (h *harness) shot(t *testing.T, args map[string]any) (image.Image, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := h.session.CallTool(ctx, &mcp.CallToolParams{Name: "screenshot", Arguments: args})
	if err != nil || res.IsError {
		t.Fatalf("screenshot failed: %v %+v", err, res)
	}
	imgContent, ok := res.Content[0].(*mcp.ImageContent)
	if !ok || imgContent.MIMEType != "image/png" {
		t.Fatalf("first content should be a PNG image: %+v", res.Content[0])
	}
	img, err := png.Decode(bytes.NewReader(imgContent.Data))
	if err != nil {
		t.Fatal(err)
	}
	return img, res.Content[1].(*mcp.TextContent).Text
}

// boxOf lê a caixa de um elemento pela saída do inspect.
func boxOf(t *testing.T, inspect string) image.Rectangle {
	t.Helper()
	var x, y, w, hh int
	for _, line := range strings.Split(inspect, "\n") {
		if _, err := fmt.Sscanf(line, "box: x=%d y=%d w=%d h=%d", &x, &y, &w, &hh); err == nil {
			return image.Rect(x, y, x+w, y+hh)
		}
	}
	t.Fatalf("no box in:\n%s", inspect)
	return image.Rectangle{}
}

func rgba(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func near(a, b color.RGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) < 40 && d(a.G, b.G) < 40 && d(a.B, b.B) < 40
}

var markPink = color.RGBA{230, 0, 120, 255}

// Critério de aceite da Fase 2: refs do print batem com o snapshot e inspect lê as cores certas.
func TestScreenshotMarksMatchSnapshot(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/styles.html")})
	button := ref(t, snap, "button", "Salvar")
	info := h.ok(t, "inspect", map[string]any{"ref": button})
	box := boxOf(t, info)

	plain, _ := h.shot(t, map[string]any{"marks": false})
	marked, caption := h.shot(t, map[string]any{})
	mustContain(t, caption, "marks with the same refs as the snapshot")
	if marked.Bounds() != plain.Bounds() || marked.Bounds().Dx() != 1280 {
		t.Fatalf("unexpected sizes: %v vs %v", marked.Bounds(), plain.Bounds())
	}

	// A borda da marca fica exatamente sobre a borda do botão informada pelo snapshot.
	left := image.Pt(box.Min.X, box.Min.Y+box.Dy()/2)
	if got := rgba(marked.At(left.X, left.Y)); got != markPink {
		t.Fatalf("mark border at %v = %v, want %v", left, got, markPink)
	}
	if got := rgba(plain.At(left.X, left.Y)); got == markPink {
		t.Fatal("unmarked screenshot should not have the mark color")
	}
	// Dentro do botão a cor real (#0055aa) continua visível.
	center := image.Pt(box.Min.X+box.Dx()/2, box.Min.Y+3)
	if got := rgba(marked.At(center.X, center.Y)); !near(got, color.RGBA{0, 85, 170, 255}) {
		t.Fatalf("button background at %v = %v", center, got)
	}
	// Etiqueta com a ref logo acima da caixa.
	label := image.Pt(box.Min.X+1, box.Min.Y-2)
	if got := rgba(marked.At(label.X, label.Y)); !near(got, markPink) {
		t.Fatalf("label background at %v = %v", label, got)
	}
}

func TestInspectColors(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/styles.html")})
	out := h.ok(t, "inspect", map[string]any{"ref": ref(t, snap, "button", "Salvar"), "props": []string{"outline-style"}})
	mustContain(t, out,
		"color: #ffffff",
		"background: #0055aa",
		"font: 700 16px",
		"border: 2px solid #003377; radius: 6px",
		"cursor: pointer",
		`aria: aria-describedby="dica"`,
		"outline-style: none",
	)

	out = h.ok(t, "inspect", map[string]any{"ref": ref(t, snap, "status", "Salva o rascunho")})
	mustContain(t, out, "color: #555555", "background: transparent (effective #f5f0e6 from div)")

	h.fail(t, "inspect", map[string]any{"ref": "e999"}, "ref e999 not found")
}

func TestScreenshotCropAndFullPage(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/styles.html")})
	button := ref(t, snap, "button", "Salvar")
	box := boxOf(t, h.ok(t, "inspect", map[string]any{"ref": button}))

	crop, caption := h.shot(t, map[string]any{"ref": button})
	mustContain(t, caption, "element "+button)
	if crop.Bounds().Dx() != box.Dx()+32 || crop.Bounds().Dy() != box.Dy()+32 {
		t.Fatalf("crop %v should be the box %v plus 16px margin", crop.Bounds(), box)
	}

	full, _ := h.shot(t, map[string]any{"fullPage": true})
	if full.Bounds().Dy() < 1800 {
		t.Fatalf("full page should include the tall section, got %v", full.Bounds())
	}
	// O link do fim da página também recebe marca, na posição do documento.
	all := h.ok(t, "snapshot", map[string]any{"full": true})
	link := boxOf(t, h.ok(t, "inspect", map[string]any{"ref": ref(t, all, "link", "Voltar ao topo")}))
	if got := rgba(full.At(link.Min.X, link.Min.Y+link.Dy()/2)); got != markPink {
		t.Fatalf("footer link mark at %v = %v", link, got)
	}
}
