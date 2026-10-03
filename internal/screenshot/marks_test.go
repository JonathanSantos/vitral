package screenshot

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/jors/browser-mcp/internal/snapshot"
)

func white(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
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
	if marks[0].Ref != "e2" || marks[0].Rect != image.Rect(10, 120, 40, 160) || marks[0].Kind != KindAction {
		t.Fatalf("unexpected mark %+v", marks[0])
	}
	if marks[1].Kind != KindText {
		t.Fatalf("heading should be text kind: %+v", marks[1])
	}
}

func TestAnnotate(t *testing.T) {
	src := white(200, 100)
	m := Mark{Ref: "e12", Rect: image.Rect(50, 40, 150, 80), Kind: KindAction}
	out := Annotate(src, []Mark{m})

	if out.Bounds() != src.Bounds() {
		t.Fatalf("dimensions changed: %v", out.Bounds())
	}
	pink := palette[KindAction]
	tests := []struct {
		name string
		x, y int
		want color.RGBA
	}{
		{"left border", 50, 60, pink},
		{"right border", 149, 60, pink},
		{"bottom border", 100, 79, pink},
		{"inside untouched", 100, 60, color.RGBA{255, 255, 255, 255}},
		{"outside untouched", 10, 10, color.RGBA{255, 255, 255, 255}},
	}
	for _, tt := range tests {
		if got := out.RGBAAt(tt.x, tt.y); got != tt.want {
			t.Errorf("%s at (%d,%d) = %v, want %v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
	if src.RGBAAt(50, 60) != (color.RGBA{255, 255, 255, 255}) {
		t.Error("source image must not be modified")
	}

	// Etiqueta acima da caixa: fundo rosa com texto branco dentro.
	label := LabelRect(m, src.Bounds())
	if label != image.Rect(50, 10, 50+(3*7+4)*2, 40) {
		t.Fatalf("label rect = %v", label)
	}
	hasText := false
	for x := label.Min.X; x < label.Max.X; x++ {
		for y := label.Min.Y; y < label.Max.Y; y++ {
			if out.RGBAAt(x, y).R == 255 && out.RGBAAt(x, y).G == 255 {
				hasText = true
			}
		}
	}
	if !hasText {
		t.Error("label should contain white text pixels")
	}
}

func TestLabelRectClamps(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 100)
	tests := []struct {
		name string
		mark image.Rectangle
		want image.Rectangle
	}{
		{"top edge goes inside", image.Rect(10, 0, 50, 20), image.Rect(10, 0, 10+36, 30)},
		{"right edge shifts left", image.Rect(95, 50, 99, 60), image.Rect(100-36, 20, 100, 50)},
	}
	for _, tt := range tests {
		if got := LabelRect(Mark{Ref: "e1", Rect: tt.mark}, bounds); got != tt.want {
			t.Errorf("%s: got %v want %v", tt.name, got, tt.want)
		}
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
