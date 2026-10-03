package snapshot

import (
	"strings"
	"testing"
)

func TestColorHex(t *testing.T) {
	tests := map[string]string{
		"rgb(255, 255, 255)":    "#ffffff",
		"rgb(0, 85, 170)":       "#0055aa",
		"rgba(0, 0, 0, 0)":      "transparent",
		"rgba(0, 85, 170, 0.5)": "#0055aa80",
		"rgba(0, 85, 170, 1)":   "#0055aa",
		"rgb(0 85 170 / 50%)":   "#0055aa80",
		"oklch(0.7 0.1 200)":    "oklch(0.7 0.1 200)",
		" rgb(12.4, 12.6, 0) ":  "#0c0d00",
	}
	for in, want := range tests {
		if got := ColorHex(in); got != want {
			t.Errorf("ColorHex(%q)=%q want %q", in, got, want)
		}
	}
}

func TestFormatInspection(t *testing.T) {
	in := &Inspection{
		Ref: "e3", Role: "button", Name: "Salvar", Tag: "button", Box: [4]int{10, 20, 100, 32},
		Styles: map[string]string{
			"color": "rgb(255, 255, 255)", "background-color": "rgba(0, 0, 0, 0)", "font-weight": "600",
			"font-size": "14px", "font-style": "normal", "font-family": "Inter", "line-height": "20px",
			"border-top-width": "1px", "border-top-style": "solid", "border-top-color": "rgb(0, 51, 119)",
			"border-right-width": "1px", "border-bottom-width": "1px", "border-left-width": "1px",
			"border-radius": "4px", "padding": "6px 12px", "margin": "0px", "opacity": "1", "cursor": "pointer",
			"display": "inline-block", "box-shadow": "none",
		},
		Attrs:  map[string]string{"type": "submit", "class": "btn"},
		Aria:   map[string]string{"aria-describedby": "dica"},
		States: []string{"focused"},
		Extra:  map[string]string{"outline-color": "rgb(255, 0, 0)"},
		Text:   "Salvar",
	}
	in.Background.Color, in.Background.Source = "rgb(0, 85, 170)", "e2"
	got := FormatInspection(in)
	for _, want := range []string{
		`[e3] button "Salvar" <button class="btn" type="submit">`,
		"box: x=10 y=20 w=100 h=32",
		"color: #ffffff",
		"background: transparent (effective #0055aa from e2)",
		"font: 600 14px Inter; line-height: 20px",
		"border: 1px solid #003377; radius: 4px",
		"states: focused",
		`aria: aria-describedby="dica"`,
		"outline-color: #ff0000",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "text:") {
		t.Errorf("text equal to name should be omitted:\n%s", got)
	}
}
