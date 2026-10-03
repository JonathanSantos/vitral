// Package visual compara imagens com goldens PNG dentro de uma tolerância (docs/TESTING.md, seção 10):
// até 8 de diferença por canal conta como igual, e até 0,5% dos pixels podem passar disso.
package visual

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// Tolerance é o limite da comparação.
type Tolerance struct {
	Channel  uint8   // diferença máxima por canal para o pixel contar como igual
	Fraction float64 // fração máxima de pixels diferentes
}

// Default é a tolerância da estratégia de testes.
var Default = Tolerance{Channel: 8, Fraction: 0.005}

// Result é o resultado de uma comparação.
type Result struct {
	Size  bool // tamanhos diferentes (nada mais é comparado)
	Diff  int  // pixels fora da tolerância por canal
	Total int
	Mask  *image.RGBA // pixels diferentes em vermelho sobre a imagem esmaecida
}

// Fraction é a fração de pixels diferentes.
func (r Result) Fraction() float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.Diff) / float64(r.Total)
}

// OK diz se a comparação passa na tolerância.
func (r Result) OK(t Tolerance) bool { return !r.Size && r.Fraction() <= t.Fraction }

func (r Result) String() string {
	if r.Size {
		return "image size differs"
	}
	return fmt.Sprintf("%d of %d pixels differ (%.3f%%)", r.Diff, r.Total, 100*r.Fraction())
}

// Compare compara got com want pixel a pixel.
func Compare(got, want image.Image, t Tolerance) Result {
	gb, wb := got.Bounds(), want.Bounds()
	if gb.Dx() != wb.Dx() || gb.Dy() != wb.Dy() {
		return Result{Size: true}
	}
	res := Result{Total: gb.Dx() * gb.Dy(), Mask: image.NewRGBA(image.Rect(0, 0, gb.Dx(), gb.Dy()))}
	for y := 0; y < gb.Dy(); y++ {
		for x := 0; x < gb.Dx(); x++ {
			g := toRGBA(got.At(gb.Min.X+x, gb.Min.Y+y))
			w := toRGBA(want.At(wb.Min.X+x, wb.Min.Y+y))
			if far(g.R, w.R, t.Channel) || far(g.G, w.G, t.Channel) || far(g.B, w.B, t.Channel) || far(g.A, w.A, t.Channel) {
				res.Diff++
				res.Mask.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
				continue
			}
			res.Mask.SetRGBA(x, y, color.RGBA{w.R/4 + 191, w.G/4 + 191, w.B/4 + 191, 255})
		}
	}
	return res
}

func toRGBA(c color.Color) color.RGBA {
	return color.RGBAModel.Convert(c).(color.RGBA)
}

func far(a, b, tol uint8) bool {
	if a > b {
		return a-b > tol
	}
	return b-a > tol
}

// Golden compara img com o golden em path. Com update, grava o golden. Se falhar, grava
// <nome>.got.png e <nome>.diff.png em outDir para inspeção e devolve o erro.
func Golden(path string, img image.Image, update bool, outDir string) error {
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		return writePNG(path, img)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read golden %s (run with -update to create it): %w", path, err)
	}
	want, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode golden %s: %w", path, err)
	}
	res := Compare(img, want, Default)
	if res.OK(Default) {
		return nil
	}
	base := strings.TrimSuffix(filepath.Base(path), ".png")
	if outDir != "" && os.MkdirAll(outDir, 0o750) == nil {
		_ = writePNG(filepath.Join(outDir, base+".got.png"), img)
		if res.Mask != nil {
			_ = writePNG(filepath.Join(outDir, base+".diff.png"), res.Mask)
		}
	}
	return fmt.Errorf("%s: %s, tolerance is %.1f%% (artifacts in %s; run with -update to accept)",
		path, res, 100*Default.Fraction, outDir)
}

func writePNG(path string, img image.Image) error {
	rgba := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}
