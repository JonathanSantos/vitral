package ring

import (
	"slices"
	"testing"
)

func TestBuffer(t *testing.T) {
	tests := []struct {
		name        string
		capacity    int
		push        []int
		want        []int
		wantEvicted []int
	}{
		{name: "under capacity", capacity: 3, push: []int{1, 2}, want: []int{1, 2}},
		{name: "exact", capacity: 3, push: []int{1, 2, 3}, want: []int{1, 2, 3}},
		{name: "wraps", capacity: 3, push: []int{1, 2, 3, 4, 5}, want: []int{3, 4, 5}, wantEvicted: []int{1, 2}},
		{name: "min capacity", capacity: 0, push: []int{1, 2}, want: []int{2}, wantEvicted: []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New[int](tt.capacity)
			var evicted []int
			for _, v := range tt.push {
				if e, ok := b.Push(v); ok {
					evicted = append(evicted, e)
				}
			}
			if got := b.Items(); !slices.Equal(got, tt.want) {
				t.Fatalf("items = %v, want %v", got, tt.want)
			}
			if !slices.Equal(evicted, tt.wantEvicted) {
				t.Fatalf("evicted = %v, want %v", evicted, tt.wantEvicted)
			}
			b.Clear()
			if b.Len() != 0 || len(b.Items()) != 0 {
				t.Fatal("clear failed")
			}
		})
	}
}
