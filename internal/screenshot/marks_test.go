package screenshot

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/JonathanSantos/vitral/internal/brand"
	"github.com/JonathanSantos/vitral/internal/snapshot"
)

var paperWhite = color.RGBA{255, 255, 255, 255}

func white(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(paperWhite), image.Point{}, draw.Src)
	return img
}

func TestMarksFrom(t *testing.T) {
	nodes := []*snapshot.Node{
		{Ref: "e1", Role: "main", Box: [4]int{0, 0, 800, 600}, Children: []*snapshot.Node{
			{Ref: "e2", Role: "button", Box: [4]int{10, 20, 30, 40}},
			{Ref: "e3", Role: "heading", Box: [4]int{0, 0, 100, 20}},
			{Ref: "e4", Role: "option", Box: [4]int{0, 0, 0, 0}},
		}},
	}
	marks := MarksFrom(nodes, image.Pt(0, 100))
	if len(marks) != 2 {
		t.Fatalf("want 2 marks (container and empty box skipped), got %+v", marks)
	}
	if marks[0].Ref != "e2" || marks[0].Rect != image.Rect(10, 120, 40, 160) {
		t.Fatalf("unexpected mark %+v", marks[0])
	}
}

func TestAnnotateOutline(t *testing.T) {
	src := white(300, 200)
	m := Mark{Ref: "e2", Rect: image.Rect(100, 80, 200, 120)} // e2 = Rubi
	out := Annotate(src, []Mark{m})
	ruby, _ := brand.MarkColor(2)

	if out.Bounds() != src.Bounds() {
		t.Fatalf("dimensions changed: %v", out.Bounds())
	}
	tests := []struct {
		name string
		x, y int
		want color.RGBA
	}{
		{"left stroke, 6px outside the box", 94, 100, ruby},
		{"left stroke inner edge", 96, 100, ruby},
		{"gap between stroke and box", 98, 100, paperWhite},
		{"inside the box untouched", 150, 100, paperWhite},
		{"right stroke", 205, 100, ruby},
		{"bottom stroke", 150, 125, ruby},
		{"rounded corner stays clear", 94, 125, paperWhite},
		{"outside untouched", 10, 10, paperWhite},
	}
	for _, tt := range tests {
		if got := out.RGBAAt(tt.x, tt.y); got != tt.want {
			t.Errorf("%s at (%d,%d) = %v, want %v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
	if src.RGBAAt(94, 100) != paperWhite {
		t.Error("source image must not be modified")
	}
}

func TestLabelColorsAndPosition(t *testing.T) {
	tests := []struct {
		ref      string
		textDark bool
	}{
		{"e1", false}, // Safira: texto branco
		{"e2", true},  // Rubi: texto Chumbo
		{"e5", false}, // Ametista: texto branco
	}
	for _, tt := range tests {
		m := Mark{Ref: tt.ref, Rect: image.Rect(100, 80, 200, 120)}
		out := Annotate(white(300, 200), []Mark{m})
		label := LabelRect(m, out.Bounds())

		// A etiqueta fica no canto superior esquerdo, cruzando a linha do contorno.
		if label.Min.X != 94 || label.Min.Y >= 74 || label.Max.Y <= 74 {
			t.Fatalf("%s label rect = %v", tt.ref, label)
		}
		if label.Dy() != labelH || label.Dx() < 30 {
			t.Fatalf("%s label size = %v", tt.ref, label.Size())
		}
		fill, text := brand.MarkColor(brand.RefNumber(tt.ref))
		var sawFill, sawText, sawBlend bool
		for x := label.Min.X; x < label.Max.X; x++ {
			for y := label.Min.Y; y < label.Max.Y; y++ {
				switch c := out.RGBAAt(x, y); {
				case c == fill:
					sawFill = true
				case c == text:
					sawText = true
				case c != paperWhite:
					sawBlend = true
				}
			}
		}
		if !sawFill || !sawText {
			t.Errorf("%s label should have fill %v and text %v", tt.ref, fill, text)
		}
		// Go Mono com antialias: as bordas das letras misturam texto e fundo (a bitmap não tinha isso).
		if !sawBlend {
			t.Errorf("%s label text should be anti-aliased", tt.ref)
		}
		if (text == brand.Ink) != tt.textDark {
			t.Errorf("%s text color = %v", tt.ref, text)
		}
	}
}

func TestLabelRectsAvoidOverlap(t *testing.T) {
	bounds := image.Rect(0, 0, 400, 400)
	a := Mark{Ref: "e1", Rect: image.Rect(100, 100, 200, 140)}
	b := Mark{Ref: "e2", Rect: image.Rect(110, 104, 180, 130)} // etiqueta cairia em cima da de e1
	rects := LabelRects([]Mark{a, b}, bounds)
	if rects[0] != LabelRect(a, bounds) {
		t.Fatalf("first label must stay on top: %v", rects[0])
	}
	if rects[1].Overlaps(rects[0]) || rects[1].Min.Y != Outline(b).Max.Y {
		t.Fatalf("second label should move below its outline: %v (outline %v)", rects[1], Outline(b))
	}
}

func TestLabelRectClamps(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 100)
	r := LabelRect(Mark{Ref: "e1", Rect: image.Rect(2, 0, 50, 20)}, bounds)
	if !r.In(bounds) {
		t.Fatalf("label must stay inside the image: %v", r)
	}
	r = LabelRect(Mark{Ref: "e1", Rect: image.Rect(95, 50, 99, 60)}, bounds)
	if r.Max.X != 100 {
		t.Fatalf("label at right edge should shift left: %v", r)
	}
}

func TestVisibleUsesOutline(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 100)
	marks := []Mark{
		{Ref: "e1", Rect: image.Rect(10, 10, 20, 20)},
		{Ref: "e2", Rect: image.Rect(103, 10, 110, 20)}, // só o contorno entra na imagem
		{Ref: "e3", Rect: image.Rect(200, 10, 210, 20)},
	}
	if got := Visible(marks, bounds); len(got) != 2 {
		t.Fatalf("visible = %+v", got)
	}
}

func TestToCSSPixelsAndCrop(t *testing.T) {
	scaled := ToCSSPixels(white(400, 200), 2)
	if scaled.Bounds().Dx() != 200 || scaled.Bounds().Dy() != 100 {
		t.Fatalf("scaled bounds %v", scaled.Bounds())
	}
	if same := ToCSSPixels(white(10, 10), 1); same.Bounds().Dx() != 10 {
		t.Fatal("dpr 1 must keep size")
	}
	crop := Crop(white(200, 100), image.Rect(50, 40, 60, 50), 5)
	if crop.Bounds() != image.Rect(0, 0, 20, 20) {
		t.Fatalf("crop bounds %v", crop.Bounds())
	}
	edge := Crop(white(200, 100), image.Rect(0, 0, 10, 10), 5)
	if edge.Bounds() != image.Rect(0, 0, 15, 15) {
		t.Fatalf("crop must clamp at the edges: %v", edge.Bounds())
	}
	data, err := Encode(crop)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(data)
	if err != nil || back.Bounds() != crop.Bounds() {
		t.Fatalf("roundtrip failed: %v %v", err, back.Bounds())
	}
}
