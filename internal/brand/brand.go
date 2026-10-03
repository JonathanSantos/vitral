// Package brand é a fonte única das cores da marca Vitral (docs/BRAND.md, seção 5).
// Nenhum outro pacote declara hex: o screenshot e o CSS injetado leem daqui.
package brand

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// Cores da marca.
var (
	Ink       = color.RGBA{R: 0x1E, G: 0x1B, B: 0x2E, A: 0xFF} // Chumbo
	Sapphire  = color.RGBA{R: 0x3E, G: 0x63, B: 0xDD, A: 0xFF} // Safira (primária)
	Ruby      = color.RGBA{R: 0xE5, G: 0x48, B: 0x4D, A: 0xFF} // Rubi
	Emerald   = color.RGBA{R: 0x30, G: 0xA4, B: 0x6C, A: 0xFF} // Esmeralda
	Amber     = color.RGBA{R: 0xF5, G: 0xA5, B: 0x24, A: 0xFF} // Âmbar
	Amethyst  = color.RGBA{R: 0x8E, G: 0x4E, B: 0xC6, A: 0xFF} // Ametista
	Paper     = color.RGBA{R: 0xF7, G: 0xF5, B: 0xF0, A: 0xFF} // Vidro
	Muted     = color.RGBA{R: 0x5B, G: 0x57, B: 0x70, A: 0xFF} // texto secundário
	White     = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	Primary   = Sapphire
	markOrder = []color.RGBA{Sapphire, Ruby, Emerald, Amber, Amethyst}
)

// MarkColor devolve a cor de preenchimento da marcação da ref N e a cor do texto da etiqueta.
// A cor é estável: eN usa marks[(N-1) % 5]. Texto branco sobre Safira e Ametista, Chumbo nas demais.
func MarkColor(ref int) (fill, text color.RGBA) {
	fill = markOrder[MarkIndex(ref)-1]
	if fill == Sapphire || fill == Amethyst {
		return fill, White
	}
	return fill, Ink
}

// MarkIndex é a posição (1 a 5) da cor da ref N na ordem das marcações; o CSS usa --vitral-markI.
func MarkIndex(ref int) int {
	if ref < 1 {
		ref = 1
	}
	return (ref-1)%len(markOrder) + 1
}

// RefNumber extrai N de "eN" (0 se não for uma ref).
func RefNumber(ref string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(ref, "e"))
	if err != nil || !strings.HasPrefix(ref, "e") {
		return 0
	}
	return n
}

// Hex formata uma cor como #rrggbb.
func Hex(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// CSSVars gera as custom properties usadas pelo CSS injetado na página.
func CSSVars() string {
	vars := []struct {
		name string
		c    color.RGBA
	}{
		{"ink", Ink}, {"primary", Sapphire}, {"mark1", Sapphire}, {"mark2", Ruby}, {"mark3", Emerald},
		{"mark4", Amber}, {"mark5", Amethyst}, {"paper", Paper}, {"muted", Muted}, {"white", White},
	}
	var b strings.Builder
	for _, v := range vars {
		fmt.Fprintf(&b, "--vitral-%s:%s;", v.name, Hex(v.c))
	}
	return b.String()
}
