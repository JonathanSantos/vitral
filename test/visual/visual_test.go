package visual

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestCompare(t *testing.T) {
	gray := color.RGBA{100, 100, 100, 255}
	tests := []struct {
		name   string
		mutate func(*image.RGBA)
		diff   int
		ok     bool
	}{
		{"identical", func(*image.RGBA) {}, 0, true},
		{"8 per channel is equal", func(img *image.RGBA) { img.SetRGBA(0, 0, color.RGBA{108, 92, 108, 255}) }, 0, true},
		{"9 per channel differs", func(img *image.RGBA) { img.SetRGBA(0, 0, color.RGBA{109, 100, 100, 255}) }, 1, true},
		{"alpha counts", func(img *image.RGBA) { img.SetRGBA(0, 0, color.RGBA{100, 100, 100, 200}) }, 1, true},
		// 100x100 = 10000 pixels: 50 diferentes é exatamente 0,5%, 51 passa do limite.
		{"0.5% passes", func(img *image.RGBA) { paint(img, 50) }, 50, true},
		{"over 0.5% fails", func(img *image.RGBA) { paint(img, 51) }, 51, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := solid(100, 100, gray)
			got := solid(100, 100, gray)
			tt.mutate(got)
			res := Compare(got, want, Default)
			if res.Diff != tt.diff || res.OK(Default) != tt.ok {
				t.Fatalf("diff=%d ok=%v, want diff=%d ok=%v (%s)", res.Diff, res.OK(Default), tt.diff, tt.ok, res)
			}
		})
	}
}

func paint(img *image.RGBA, n int) {
	for i := 0; i < n; i++ {
		img.SetRGBA(i%100, i/100, color.RGBA{255, 0, 0, 255})
	}
}

func TestCompareSizeAndOffsetBounds(t *testing.T) {
	gray := color.RGBA{1, 2, 3, 255}
	res := Compare(solid(10, 10, gray), solid(10, 11, gray), Default)
	if !res.Size || res.OK(Default) || res.String() != "image size differs" {
		t.Fatalf("size mismatch not reported: %+v", res)
	}
	// Imagens recortadas (Min != 0) são comparadas pela posição relativa.
	cropped := solid(20, 20, gray).SubImage(image.Rect(5, 5, 15, 15))
	if res := Compare(cropped, solid(10, 10, gray), Default); !res.OK(Default) || res.Fraction() != 0 {
		t.Fatalf("cropped image should match: %s", res)
	}
	if (Result{}).Fraction() != 0 {
		t.Fatal("empty result fraction")
	}
}

func TestGolden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "golden", "a.png")
	out := filepath.Join(dir, "out")
	img := solid(10, 10, color.RGBA{10, 20, 30, 255})

	if err := Golden(path, img, false, out); err == nil || !strings.Contains(err.Error(), "-update") {
		t.Fatalf("missing golden must ask for -update: %v", err)
	}
	if err := Golden(path, img, true, out); err != nil {
		t.Fatal(err)
	}
	if err := Golden(path, img, false, out); err != nil {
		t.Fatalf("same image should pass: %v", err)
	}
	other := solid(10, 10, color.RGBA{200, 20, 30, 255})
	err := Golden(path, other, false, out)
	if err == nil || !strings.Contains(err.Error(), "100 of 100 pixels differ") {
		t.Fatalf("different image should fail with the count: %v", err)
	}
	for _, f := range []string{"a.got.png", "a.diff.png"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Fatalf("artifact %s not written: %v", f, err)
		}
	}
	if err := os.WriteFile(path, []byte("not png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Golden(path, img, false, out); err == nil || !strings.Contains(err.Error(), "decode golden") {
		t.Fatalf("corrupt golden: %v", err)
	}
}
