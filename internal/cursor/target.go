package cursor

import (
	"math"
	"math/rand/v2"
	"sort"
)

// Kind é o tipo de alvo, que decide o ponto de clique e a forma do cursor ao chegar.
type Kind int

const (
	Arrow   Kind = iota // padrão
	Pointer             // cursor computado "pointer" (links, botões)
	Text                // campo de texto ou cursor "text"
)

func (k Kind) String() string {
	switch k {
	case Pointer:
		return "pointer"
	case Text:
		return "text"
	}
	return "arrow"
}

// Candidates sorteia os pontos de clique para a página validar numa ida só: Tries sorteios e, por
// último, o ponto de repouso (o centro; num campo de texto, o centro do terço esquerdo). Sorteia
// sempre todos, para a mesma semente gerar o mesmo plano qualquer que seja o candidato aceito.
func Candidates(target Rect, kind Kind, p Profile, rng *rand.Rand) []Point {
	target = saneRect(target)
	inner := target.Inset(p.Inner)
	out := make([]Point, 0, p.Tries+1)
	area := inner
	if kind == Text {
		area.W /= 3
	}
	for i := 0; i < p.Tries; i++ {
		out = append(out, roundInside(sample(inner, kind, p, rng), area))
	}
	return append(out, roundInside(rest(inner, kind), area))
}

// roundInside arredonda para 0,1 px sem sair da área: o arredondamento comum poderia empurrar o
// ponto para fora num alvo minúsculo. Área menor que 0,1 px fica com o centro arredondado.
func roundInside(pt Point, area Rect) Point {
	axis := func(v, lo, size float64) float64 {
		a, b := math.Ceil(lo*10)/10, math.Floor((lo+size)*10)/10
		if a > b {
			return round1(lo + size/2)
		}
		return clamp(round1(v), a, b)
	}
	return Point{axis(pt.X, area.X, area.W), axis(pt.Y, area.Y, area.H)}
}

// sample é a normal em torno do ponto de repouso, limitada à parte interna do alvo (4.1).
func sample(inner Rect, kind Kind, p Profile, rng *rand.Rand) Point {
	c := rest(inner, kind)
	sx, sy := p.Sigma*inner.W/p.Inner, p.Sigma*inner.H/p.Inner // σ em fração do alvo inteiro
	if kind == Text {
		sx, sy = inner.W/18, inner.H/20 // terço esquerdo, meio vertical
	}
	x := c.X + truncNormal(rng, sx, 3*sx)
	y := c.Y + truncNormal(rng, sy, 3*sy)
	left, right := inner.X, inner.X+inner.W
	if kind == Text {
		right = inner.X + inner.W/3
	}
	return Point{clamp(x, left, right), clamp(y, inner.Y, inner.Y+inner.H)}
}

func rest(inner Rect, kind Kind) Point {
	if kind == Text {
		return Point{inner.X + inner.W/6, inner.Y + inner.H/2}
	}
	return inner.Center()
}

// FreeArea acha o maior retângulo dentro do alvo que não encosta em nenhum obstáculo (os filhos
// interativos do alvo). É o último recurso quando nenhum candidato acerta só o alvo: o centro desse
// retângulo é um ponto que clica no alvo e não num filho. ok=false se não sobra área livre.
func FreeArea(target Rect, obstacles []Rect) (Rect, bool) {
	target = saneRect(target)
	if target.W <= 0 || target.H <= 0 {
		return Rect{}, false
	}
	xs := []float64{target.X, target.X + target.W}
	ys := []float64{target.Y, target.Y + target.H}
	var obs []Rect
	for _, o := range obstacles {
		o = saneRect(o)
		x0, y0 := clamp(o.X, target.X, target.X+target.W), clamp(o.Y, target.Y, target.Y+target.H)
		x1, y1 := clamp(o.X+o.W, target.X, target.X+target.W), clamp(o.Y+o.H, target.Y, target.Y+target.H)
		if x1 <= x0 || y1 <= y0 {
			continue
		}
		obs = append(obs, Rect{x0, y0, x1 - x0, y1 - y0})
		xs = append(xs, x0, x1)
		ys = append(ys, y0, y1)
	}
	xs, ys = uniq(xs), uniq(ys)
	cols, rows := len(xs)-1, len(ys)-1
	// free[r][c]: a célula da grade (entre linhas consecutivas) não está dentro de obstáculo.
	free := make([][]bool, rows)
	for r := range free {
		free[r] = make([]bool, cols)
		for c := range free[r] {
			mid := Point{(xs[c] + xs[c+1]) / 2, (ys[r] + ys[r+1]) / 2}
			free[r][c] = true
			for _, o := range obs {
				if mid.X > o.X && mid.X < o.X+o.W && mid.Y > o.Y && mid.Y < o.Y+o.H {
					free[r][c] = false
					break
				}
			}
		}
	}
	// Maior retângulo em histograma, linha a linha, com colunas de larguras diferentes. start[c] é a
	// primeira linha livre seguida da coluna; as bordas saem sempre da grade (sem erro de arredondamento).
	start := make([]int, cols)
	run := make([]bool, cols)
	var best Rect
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if !free[r][c] {
				run[c] = false
				continue
			}
			if !run[c] {
				run[c], start[c] = true, r
			}
		}
		for c0 := 0; c0 < cols; c0++ {
			top := -1
			for c1 := c0; c1 < cols && run[c1]; c1++ {
				top = max(top, start[c1])
				w, h := xs[c1+1]-xs[c0], ys[r+1]-ys[top]
				if w*h > best.W*best.H {
					best = Rect{xs[c0], ys[top], w, h}
				}
			}
		}
	}
	return best, best.W > 0 && best.H > 0
}

func uniq(v []float64) []float64 {
	sort.Float64s(v)
	out := v[:1]
	for _, x := range v[1:] {
		if x > out[len(out)-1] {
			out = append(out, x)
		}
	}
	return out
}
