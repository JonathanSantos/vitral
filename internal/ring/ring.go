// Package ring implementa um buffer circular simples. Não é thread-safe: o dono sincroniza.
package ring

// Buffer guarda os últimos N itens.
type Buffer[T any] struct {
	items []T
	start int
	size  int
}

// New cria um buffer com capacidade fixa (mínimo 1).
func New[T any](capacity int) *Buffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &Buffer[T]{items: make([]T, capacity)}
}

// Push adiciona um item e devolve o item descartado, se houver.
func (b *Buffer[T]) Push(v T) (evicted T, ok bool) {
	capacity := len(b.items)
	if b.size < capacity {
		b.items[(b.start+b.size)%capacity] = v
		b.size++
		return evicted, false
	}
	evicted = b.items[b.start]
	b.items[b.start] = v
	b.start = (b.start + 1) % capacity
	return evicted, true
}

// Items devolve uma cópia, do mais antigo para o mais novo.
func (b *Buffer[T]) Items() []T {
	out := make([]T, b.size)
	for i := range b.size {
		out[i] = b.items[(b.start+i)%len(b.items)]
	}
	return out
}

// Len devolve a quantidade de itens.
func (b *Buffer[T]) Len() int { return b.size }

// Clear esvazia o buffer.
func (b *Buffer[T]) Clear() {
	clear(b.items)
	b.start, b.size = 0, 0
}
