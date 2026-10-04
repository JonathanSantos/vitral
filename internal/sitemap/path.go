package sitemap

import (
	"container/heap"
	"math"
	"time"

	"github.com/JonathanSantos/vitral/internal/heal"
)

// weight é o custo de usar a aresta: quanto menos confiável (taxa de sucesso com suavização de
// Laplace) e mais antiga, mais cara; cada passo custa um pouco, para preferir caminhos curtos.
func weight(e *Edge, now time.Time) float64 {
	p := float64(e.Successes+1) / float64(e.Successes+e.Failures+2)
	age := math.Min(1, math.Max(0, now.Sub(e.LastUsed).Hours()/MaxAge.Hours()))
	return -math.Log(p) + 0.5*age + 0.05
}

// Path é o caminho mais confiável de from até to (Dijkstra com os pesos de weight), sem arestas
// suspeitas. ok=false se não há caminho; from == to dá caminho vazio.
func (m *Map) Path(from, to string, now time.Time) ([]*Edge, bool) {
	if from == to {
		return nil, m.Screen(from) != nil
	}
	dist := map[string]float64{from: 0}
	prev := map[string]*Edge{}
	done := map[string]bool{}
	pq := &queue{{id: from}}
	for pq.Len() > 0 {
		cur := heap.Pop(pq).(item)
		if done[cur.id] {
			continue
		}
		done[cur.id] = true
		if cur.id == to {
			break
		}
		for _, e := range m.Edges {
			if e.From != cur.id || e.Suspect() || e.To == e.From {
				continue
			}
			d := cur.dist + weight(e, now)
			if old, ok := dist[e.To]; !ok || d < old {
				dist[e.To], prev[e.To] = d, e
				heap.Push(pq, item{id: e.To, dist: d})
			}
		}
	}
	if _, ok := prev[to]; !ok {
		return nil, false
	}
	var path []*Edge
	for at := to; at != from; at = prev[at].From {
		path = append([]*Edge{prev[at]}, path...)
	}
	return path, true
}

type item struct {
	id   string
	dist float64
}

type queue []item

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	return q[i].dist < q[j].dist || q[i].dist == q[j].dist && q[i].id < q[j].id
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)   { *q = append(*q, x.(item)) }
func (q *queue) Pop() any     { old := *q; x := old[len(old)-1]; *q = old[:len(old)-1]; return x }

// SanitizeSig tira dados da assinatura antes de guardar: textos passam por Sanitize e o href vira
// padrão de caminho.
func SanitizeSig(s heal.Signature) heal.Signature {
	for _, f := range []*string{&s.Name, &s.Label, &s.Placeholder, &s.Heading, &s.Column, &s.RowHeader, &s.Landmark, &s.Dialog, &s.TestID, &s.NameAttr, &s.ID} {
		*f = Sanitize(*f)
	}
	if s.Href != "" && s.Href[0] == '/' {
		s.Href = normalizePath(s.Href)
	}
	return s
}
