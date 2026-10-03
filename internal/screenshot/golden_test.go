package screenshot

import (
	"flag"
	"image"
	"path/filepath"
	"testing"

	"github.com/JonathanSantos/vitral/test/visual"
)

var update = flag.Bool("update", false, "rewrite the visual goldens")

// Golden das marcações (docs/TESTING.md, seção 10): só Go puro com a Go Mono embutida, então o
// resultado é o mesmo em qualquer sistema e roda em todo go test. Cobre rótulos que colidem,
// rótulo empurrado para baixo na borda de cima, rótulo preso na borda direita e refs longas.
func TestMarksGolden(t *testing.T) {
	src := white(360, 220)
	var marks []Mark
	// Fileira de caixas de seleção pequenas e próximas: os rótulos disputam o mesmo espaço.
	for i := 0; i < 6; i++ {
		x := 20 + i*18
		marks = append(marks, Mark{Ref: "e" + string(rune('1'+i)), Rect: image.Rect(x, 60, x+12, 72)})
	}
	marks = append(marks,
		Mark{Ref: "e7", Rect: image.Rect(20, 2, 140, 30)},       // colado na borda de cima
		Mark{Ref: "e128", Rect: image.Rect(300, 100, 358, 130)}, // colado na borda direita
		Mark{Ref: "e9", Rect: image.Rect(30, 120, 250, 200)},    // caixa grande...
		Mark{Ref: "e10", Rect: image.Rect(60, 150, 140, 175)},   // ...com outra dentro
		Mark{Ref: "e1024", Rect: image.Rect(180, 20, 260, 44)},  // ref longa
	)
	out := Annotate(src, marks)
	path := filepath.Join("testdata", "visual", "marks-overlap.png")
	if err := visual.Golden(path, out, *update, filepath.Join("..", "..", ".quality", "out", "visual")); err != nil {
		t.Fatal(err)
	}
}
