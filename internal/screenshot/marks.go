// Package screenshot desenha as refs do snapshot sobre o print, sem tocar no DOM.
package screenshot

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/JonathanSantos/vitral/internal/snapshot"
)

// Mark é uma caixa a desenhar, em pixels CSS relativos à imagem.
type Mark struct {
	Ref  string
	Rect image.Rectangle
	Kind Kind
}

// Kind separa elementos acionáveis de elementos de texto/estrutura (cores diferentes).
type Kind int

const (
	KindAction Kind = iota
	KindText
)

var palette = map[Kind]color.RGBA{
	KindAction: {R: 230, G: 0, B: 120, A: 255},
	KindText:   {R: 0, G: 110, B: 220, A: 255},
}

// Papéis grandes demais para marcar: cobririam a tela inteira e esconderiam o resto.
var containerRoles = map[string]bool{
	"main": true, "banner": true, "contentinfo": true, "navigation": true, "complementary": true,
	"form": true, "region": true, "search": true, "list": true, "table": true, "grid": true,
	"group": true, "tablist": true, "menu": true, "menubar": true, "tree": true, "dialog": true,
	"row": true, "listitem": true, "tabpanel": true,
}

var textRoles = map[string]bool{"heading": true, "alert": true, "status": true, "error": true, "label": true}

// MarksFrom escolhe os nós a marcar. offset desloca as caixas (ex.: rolagem no print de página inteira).
func MarksFrom(nodes []*snapshot.Node, offset image.Point) []Mark {
	var marks []Mark
	snapshot.Walk(nodes, func(n, _ *snapshot.Node) {
		if containerRoles[n.Role] || n.Box[2] <= 0 || n.Box[3] <= 0 {
			return
		}
		kind := KindAction
		if textRoles[n.Role] {
			kind = KindText
		}
		r := image.Rect(n.Box[0], n.Box[1], n.Box[0]+n.Box[2], n.Box[1]+n.Box[3]).Add(offset)
		marks = append(marks, Mark{Ref: n.Ref, Rect: r, Kind: kind})
	})
	return marks
}

// Visible devolve só as marcas que aparecem dentro de bounds.
func Visible(marks []Mark, bounds image.Rectangle) []Mark {
	var out []Mark
	for _, m := range marks {
		if m.Rect.Overlaps(bounds) {
			out = append(out, m)
		}
	}
	return out
}

// ToCSSPixels reduz um print com devicePixelRatio > 1 para o tamanho em pixels CSS,
// para que as caixas do snapshot batam com a imagem e o print fique mais leve.
func ToCSSPixels(img image.Image, dpr float64) image.Image {
	if dpr <= 1.01 {
		return img
	}
	b := img.Bounds()
	w, h := int(float64(b.Dx())/dpr+0.5), int(float64(b.Dy())/dpr+0.5)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}

const (
	stroke     = 2
	labelPadX  = 2
	charWidth  = 7
	glyphH     = 13 + 2
	labelAlpha = 235
	// labelScale amplia a fonte bitmap 7x13: em tamanho natural ela fica ilegível para modelos de visão.
	labelScale = 2
)

// Annotate desenha caixa e etiqueta com a ref de cada marca. Não altera img.
func Annotate(img image.Image, marks []Mark) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)
	for _, m := range marks {
		c := palette[m.Kind]
		drawBorder(out, m.Rect.Intersect(b), c)
	}
	// Etiquetas depois das caixas, para não serem riscadas por bordas vizinhas.
	for _, m := range marks {
		drawLabel(out, m, b)
	}
	return out
}

func drawBorder(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	if r.Empty() {
		return
	}
	src := image.NewUniform(c)
	edges := []image.Rectangle{
		image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+stroke),
		image.Rect(r.Min.X, r.Max.Y-stroke, r.Max.X, r.Max.Y),
		image.Rect(r.Min.X, r.Min.Y, r.Min.X+stroke, r.Max.Y),
		image.Rect(r.Max.X-stroke, r.Min.Y, r.Max.X, r.Max.Y),
	}
	for _, e := range edges {
		draw.Draw(img, e.Intersect(r), src, image.Point{}, draw.Src)
	}
}

// LabelRect calcula onde fica a etiqueta: acima da caixa, ou dentro dela se não houver espaço.
func LabelRect(m Mark, bounds image.Rectangle) image.Rectangle {
	w := (len(m.Ref)*charWidth + 2*labelPadX) * labelScale
	labelH := glyphH * labelScale
	x, y := m.Rect.Min.X, m.Rect.Min.Y-labelH
	if y < bounds.Min.Y {
		y = m.Rect.Min.Y
	}
	if y < bounds.Min.Y {
		y = bounds.Min.Y
	}
	if x+w > bounds.Max.X {
		x = bounds.Max.X - w
	}
	if x < bounds.Min.X {
		x = bounds.Min.X
	}
	return image.Rect(x, y, x+w, y+labelH)
}

func drawLabel(img *image.RGBA, m Mark, bounds image.Rectangle) {
	r := LabelRect(m, bounds)
	bg := palette[m.Kind]
	bg.A = labelAlpha

	// Desenha em tamanho natural num rascunho e amplia com vizinho mais próximo (pixels nítidos).
	small := image.NewRGBA(image.Rect(0, 0, r.Dx()/labelScale, r.Dy()/labelScale))
	draw.Draw(small, small.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	d := font.Drawer{
		Dst:  small,
		Src:  image.NewUniform(color.White),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(labelPadX, basicfont.Face7x13.Ascent+1),
	}
	d.DrawString(m.Ref)
	xdraw.NearestNeighbor.Scale(img, r, small, small.Bounds(), xdraw.Over, nil)
}

// Crop recorta a imagem (com margem) em volta de r.
func Crop(img image.Image, r image.Rectangle, margin int) image.Image {
	area := r.Inset(-margin).Intersect(img.Bounds())
	if area.Empty() {
		return img
	}
	out := image.NewRGBA(image.Rect(0, 0, area.Dx(), area.Dy()))
	draw.Draw(out, out.Bounds(), img, area.Min, draw.Src)
	return out
}

// Decode e Encode convertem PNG.
func Decode(data []byte) (image.Image, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}
	return img, nil
}

func Encode(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode screenshot: %w", err)
	}
	return buf.Bytes(), nil
}
