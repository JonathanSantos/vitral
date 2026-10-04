// Package cursor planeja o movimento do cursor visual da IA (docs/CURSOR.md). É só física, pura e
// determinística dada a semente: recebe posição, alvo, perfil e um *rand.Rand e devolve keyframes.
// Não conhece rod nem CDP; o overlay só interpola e desenha o que este pacote decide.
package cursor

import (
	"math"
	"math/rand/v2"
)

// Point é uma posição em CSS px da viewport.
type Point struct{ X, Y float64 }

// Rect é um retângulo em CSS px da viewport.
type Rect struct{ X, Y, W, H float64 }

// Center é o centro do retângulo.
func (r Rect) Center() Point { return Point{r.X + r.W/2, r.Y + r.H/2} }

// Inset devolve a parte central com a fração frac da largura e da altura.
func (r Rect) Inset(frac float64) Rect {
	return Rect{r.X + r.W*(1-frac)/2, r.Y + r.H*(1-frac)/2, r.W * frac, r.H * frac}
}

// Contains inclui as bordas.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.X <= r.X+r.W && p.Y >= r.Y && p.Y <= r.Y+r.H
}

func (p Point) add(q Point) Point                  { return Point{p.X + q.X, p.Y + q.Y} }
func (p Point) sub(q Point) Point                  { return Point{p.X - q.X, p.Y - q.Y} }
func (p Point) scale(k float64) Point              { return Point{p.X * k, p.Y * k} }
func (p Point) dot(q Point) float64                { return p.X*q.X + p.Y*q.Y }
func (p Point) norm() float64                      { return math.Hypot(p.X, p.Y) }
func dist(a, b Point) float64                      { return b.sub(a).norm() }
func perp(u Point) Point                           { return Point{-u.Y, u.X} }
func clamp(v, lo, hi float64) float64              { return math.Max(lo, math.Min(hi, v)) }
func uniform(r *rand.Rand, lo, hi float64) float64 { return lo + (hi-lo)*r.Float64() }

// unit é o vetor unitário de a para b (eixo x se os pontos coincidem).
func unit(a, b Point) Point {
	d := b.sub(a)
	if n := d.norm(); n > 0 {
		return d.scale(1 / n)
	}
	return Point{1, 0}
}

// truncNormal sorteia uma normal com desvio sigma, limitada a ±limit.
func truncNormal(r *rand.Rand, sigma, limit float64) float64 {
	return clamp(r.NormFloat64()*sigma, -limit, limit)
}

// round1 arredonda para 0,1: é a precisão que vai para o overlay e para o clique real.
func round1(v float64) float64 { return math.Round(v*10) / 10 }

func roundPoint(p Point) Point { return Point{round1(p.X), round1(p.Y)} }

// maxCoord limita coordenadas absurdas (vindas de uma página quebrada) para a conta não estourar.
const maxCoord = 1e6

func finite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return clamp(v, -maxCoord, maxCoord)
}

func sanePoint(p Point) Point { return Point{finite(p.X), finite(p.Y)} }

func saneRect(r Rect) Rect {
	return Rect{finite(r.X), finite(r.Y), math.Max(finite(r.W), 0), math.Max(finite(r.H), 0)}
}
