// Package screenshot desenha as refs do snapshot sobre o print, sem tocar no DOM.
// O visual segue docs/BRAND.md (seção 5.1): cor por ref, contorno arredondado e etiqueta.
package screenshot

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/JonathanSantos/vitral/internal/brand"
	"github.com/JonathanSantos/vitral/internal/snapshot"
)

// Mark é uma caixa a desenhar, em pixels CSS relativos à imagem.
type Mark struct {
	Ref  string
	Rect image.Rectangle // bounding box do elemento
}

// Papéis grandes demais para marcar: cobririam a tela inteira e esconderiam o resto.
var containerRoles = map[string]bool{
	"main": true, "banner": true, "contentinfo": true, "navigation": true, "complementary": true,
	"form": true, "region": true, "search": true, "list": true, "table": true, "grid": true,
	"group": true, "tablist": true, "menu": true, "menubar": true, "tree": true, "dialog": true,
	"row": true, "listitem": true, "tabpanel": true,
}

// MarksFrom escolhe os nós a marcar. offset desloca as caixas (ex.: rolagem no print de página inteira).
func MarksFrom(nodes []*snapshot.Node, offset image.Point) []Mark {
	var marks []Mark
	snapshot.Walk(nodes, func(n, _ *snapshot.Node) {
		if containerRoles[n.Role] || n.Box[2] <= 0 || n.Box[3] <= 0 {
			return
		}
		r := image.Rect(n.Box[0], n.Box[1], n.Box[0]+n.Box[2], n.Box[1]+n.Box[3]).Add(offset)
		marks = append(marks, Mark{Ref: n.Ref, Rect: r})
	})
	return marks
}

// Visible devolve só as marcas cujo contorno aparece dentro de bounds.
func Visible(marks []Mark, bounds image.Rectangle) []Mark {
	var out []Mark
	for _, m := range marks {
		if Outline(m).Overlaps(bounds) {
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

// Medidas da seção 5.1 do BRAND.md.
const (
	Stroke       = 3
	OutlineGap   = 6
	OutlineRound = 6
	labelRound   = 6
	// Etiqueta em Go Mono 22 px: legível para modelos de visão e do mesmo tamanho da versão em bitmap.
	labelFontSize = 22
	labelPadX     = 6
	labelH        = 24
)

// labelFace é a Go Mono lisa (antialias). Uma opentype.Face não é segura para uso concorrente.
var (
	labelMu   sync.Mutex
	labelFace = loadLabelFace()
)

// loadLabelFace carrega a fonte embutida no x/image; se falhar, cai na fonte bitmap.
func loadLabelFace() font.Face {
	f, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return basicfont.Face7x13
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: labelFontSize, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return basicfont.Face7x13
	}
	return face
}

// Outline é o retângulo externo do contorno: a bounding box afastada OutlineGap px.
func Outline(m Mark) image.Rectangle {
	return m.Rect.Inset(-OutlineGap)
}

// Annotate desenha contorno e etiqueta de cada marca. Não altera img.
func Annotate(img image.Image, marks []Mark) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)
	for _, m := range marks {
		fill, _ := brand.MarkColor(brand.RefNumber(m.Ref))
		strokeRoundedRect(out, Outline(m), OutlineRound, Stroke, fill)
	}
	// Etiquetas depois dos contornos, para não serem riscadas por bordas vizinhas.
	for i, r := range LabelRects(marks, b) {
		drawLabel(out, marks[i].Ref, r)
	}
	return out
}

// inRoundedRect diz se o centro do pixel (x, y) está dentro de r com cantos de raio rad.
func inRoundedRect(x, y int, r image.Rectangle, rad int) bool {
	if x < r.Min.X || x >= r.Max.X || y < r.Min.Y || y >= r.Max.Y {
		return false
	}
	rad = min(rad, r.Dx()/2, r.Dy()/2)
	if rad <= 0 {
		return true
	}
	px, py := float64(x)+0.5, float64(y)+0.5
	cx, cy := px, py
	switch {
	case px < float64(r.Min.X+rad):
		cx = float64(r.Min.X + rad)
	case px > float64(r.Max.X-rad):
		cx = float64(r.Max.X - rad)
	}
	switch {
	case py < float64(r.Min.Y+rad):
		cy = float64(r.Min.Y + rad)
	case py > float64(r.Max.Y-rad):
		cy = float64(r.Max.Y - rad)
	}
	dx, dy := px-cx, py-cy
	return dx*dx+dy*dy <= float64(rad*rad)
}

// strokeRoundedRect pinta só a faixa da borda (sem varrer o interior de caixas grandes).
func strokeRoundedRect(img *image.RGBA, r image.Rectangle, rad, width int, c color.RGBA) {
	inner := r.Inset(width)
	innerRad := max(rad-width, 0)
	clip := r.Intersect(img.Bounds())
	paint := func(x, y int) {
		if inRoundedRect(x, y, r, rad) && !inRoundedRect(x, y, inner, innerRad) {
			img.SetRGBA(x, y, c)
		}
	}
	for y := clip.Min.Y; y < clip.Max.Y; y++ {
		band := y < r.Min.Y+rad+width || y >= r.Max.Y-rad-width
		if band {
			for x := clip.Min.X; x < clip.Max.X; x++ {
				paint(x, y)
			}
			continue
		}
		for x := clip.Min.X; x < min(r.Min.X+width, clip.Max.X); x++ {
			paint(x, y)
		}
		for x := max(r.Max.X-width, clip.Min.X); x < clip.Max.X; x++ {
			paint(x, y)
		}
	}
}

func fillRoundedRect(img *image.RGBA, r image.Rectangle, rad int, c color.RGBA) {
	clip := r.Intersect(img.Bounds())
	for y := clip.Min.Y; y < clip.Max.Y; y++ {
		for x := clip.Min.X; x < clip.Max.X; x++ {
			if inRoundedRect(x, y, r, rad) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func labelSize(ref string) image.Point {
	labelMu.Lock()
	defer labelMu.Unlock()
	return image.Pt(font.MeasureString(labelFace, ref).Ceil()+2*labelPadX, labelH)
}

// LabelRect posiciona a etiqueta no canto superior esquerdo do contorno, sobrepondo a linha.
func LabelRect(m Mark, bounds image.Rectangle) image.Rectangle {
	size := labelSize(m.Ref)
	o := Outline(m)
	return clampRect(image.Rectangle{Min: image.Pt(o.Min.X, o.Min.Y-size.Y/2), Max: image.Pt(o.Min.X+size.X, o.Min.Y-size.Y/2+size.Y)}, bounds)
}

// belowRect é a posição alternativa: logo abaixo do contorno, quando a de cima cruza outra etiqueta.
func belowRect(m Mark, bounds image.Rectangle) image.Rectangle {
	size := labelSize(m.Ref)
	o := Outline(m)
	return clampRect(image.Rectangle{Min: image.Pt(o.Min.X, o.Max.Y), Max: image.Pt(o.Min.X+size.X, o.Max.Y+size.Y)}, bounds)
}

func clampRect(r, bounds image.Rectangle) image.Rectangle {
	if r.Max.X > bounds.Max.X {
		r = r.Add(image.Pt(bounds.Max.X-r.Max.X, 0))
	}
	if r.Min.X < bounds.Min.X {
		r = r.Add(image.Pt(bounds.Min.X-r.Min.X, 0))
	}
	if r.Max.Y > bounds.Max.Y {
		r = r.Add(image.Pt(0, bounds.Max.Y-r.Max.Y))
	}
	if r.Min.Y < bounds.Min.Y {
		r = r.Add(image.Pt(0, bounds.Min.Y-r.Min.Y))
	}
	return r
}

// LabelRects decide a posição de todas as etiquetas: se uma cruzar outra já posicionada,
// vai para baixo do contorno do próprio elemento.
func LabelRects(marks []Mark, bounds image.Rectangle) []image.Rectangle {
	placed := make([]image.Rectangle, 0, len(marks))
	for _, m := range marks {
		r := LabelRect(m, bounds)
		for _, p := range placed {
			if r.Overlaps(p) {
				r = belowRect(m, bounds)
				break
			}
		}
		placed = append(placed, r)
	}
	return placed
}

func drawLabel(img *image.RGBA, ref string, r image.Rectangle) {
	fill, text := brand.MarkColor(brand.RefNumber(ref))
	fillRoundedRect(img, r, labelRound, fill)

	labelMu.Lock()
	defer labelMu.Unlock()
	// Centraliza verticalmente pela caixa real dos glifos (refs não têm descendentes).
	bounds, _ := font.BoundString(labelFace, ref)
	glyphTop, glyphBottom := bounds.Min.Y.Floor(), bounds.Max.Y.Ceil()
	baseline := r.Min.Y + (r.Dy()-(glyphBottom-glyphTop))/2 - glyphTop
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(text),
		Face: labelFace,
		Dot:  fixed.P(r.Min.X+labelPadX, baseline),
	}
	d.DrawString(ref)
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
