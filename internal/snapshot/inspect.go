package snapshot

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Inspection é o resultado de window.__vitral.inspect.
type Inspection struct {
	Error      string            `json:"error"`
	Ref        string            `json:"ref"`
	Role       string            `json:"role"`
	Name       string            `json:"name"`
	Tag        string            `json:"tag"`
	Box        [4]int            `json:"box"`
	Styles     map[string]string `json:"styles"`
	Extra      map[string]string `json:"extra"`
	Aria       map[string]string `json:"aria"`
	Attrs      map[string]string `json:"attrs"`
	States     []string          `json:"states"`
	Text       string            `json:"text"`
	Background struct {
		Color  string `json:"color"`
		Source string `json:"source"`
	} `json:"background"`
}

// ParseInspection decodifica o JSON do inspect.
func ParseInspection(data []byte, ref string) (*Inspection, error) {
	var in Inspection
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, fmt.Errorf("decode inspect: %w", err)
	}
	if in.Error == "not_found" {
		return nil, ErrRefNotFound{Ref: ref}
	}
	return &in, nil
}

var rgbRe = regexp.MustCompile(`^rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:\s*[,/]\s*([\d.]+%?))?\s*\)$`)

// ColorHex converte rgb()/rgba() em #rrggbb (ou #rrggbbaa com transparência). Outros formatos passam direto.
func ColorHex(css string) string {
	css = strings.TrimSpace(css)
	m := rgbRe.FindStringSubmatch(css)
	if m == nil {
		return css
	}
	channel := func(s string) int {
		f, _ := strconv.ParseFloat(s, 64)
		return int(f + 0.5)
	}
	hex := fmt.Sprintf("#%02x%02x%02x", channel(m[1]), channel(m[2]), channel(m[3]))
	if m[4] == "" {
		return hex
	}
	alpha := parseAlpha(m[4])
	if alpha >= 1 {
		return hex
	}
	if alpha == 0 {
		return "transparent"
	}
	return fmt.Sprintf("%s%02x", hex, int(alpha*255+0.5))
}

func parseAlpha(s string) float64 {
	if strings.HasSuffix(s, "%") {
		f, _ := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		return f / 100
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// FormatInspection monta a saída compacta do inspect.
func FormatInspection(in *Inspection) string {
	st := in.Styles
	var lines []string
	add := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }

	head := fmt.Sprintf("[%s] %s", in.Ref, in.Role)
	if in.Name != "" {
		head += " " + strconv.Quote(Truncate(in.Name, 80))
	}
	add("%s %s", head, tagLine(in))
	add("box: x=%d y=%d w=%d h=%d", in.Box[0], in.Box[1], in.Box[2], in.Box[3])
	add("color: %s", ColorHex(st["color"]))
	add("background: %s", background(in))
	if img := st["background-image"]; img != "" && img != "none" {
		add("background-image: %s", Truncate(img, 120))
	}
	font := strings.TrimSpace(fmt.Sprintf("%s %s %s", st["font-style"], st["font-weight"], st["font-size"]))
	font = strings.TrimPrefix(font, "normal ")
	add("font: %s %s; line-height: %s", font, st["font-family"], st["line-height"])
	add("border: %s; radius: %s", border(st), st["border-radius"])
	add("padding: %s; margin: %s", st["padding"], st["margin"])
	add("opacity: %s; cursor: %s; display: %s", st["opacity"], st["cursor"], st["display"])
	if shadow := st["box-shadow"]; shadow != "" && shadow != "none" {
		add("box-shadow: %s", Truncate(shadow, 120))
	}
	if deco := st["text-decoration-line"]; deco != "" && deco != "none" {
		add("text-decoration: %s", deco)
	}
	if t := st["text-transform"]; t != "" && t != "none" {
		add("text-transform: %s", t)
	}
	if len(in.States) > 0 {
		add("states: %s", strings.Join(in.States, ", "))
	}
	if len(in.Aria) > 0 {
		add("aria: %s", attrList(in.Aria))
	}
	for _, k := range sortedKeys(in.Extra) {
		add("%s: %s", k, ColorHex(in.Extra[k]))
	}
	if in.Text != "" && in.Text != in.Name {
		add("text: %s", strconv.Quote(in.Text))
	}
	return strings.Join(lines, "\n")
}

func tagLine(in *Inspection) string {
	if len(in.Attrs) == 0 {
		return "<" + in.Tag + ">"
	}
	return "<" + in.Tag + " " + attrList(in.Attrs) + ">"
}

func background(in *Inspection) string {
	own := ColorHex(in.Styles["background-color"])
	if own != "transparent" {
		return own
	}
	return fmt.Sprintf("transparent (effective %s from %s)", ColorHex(in.Background.Color), in.Background.Source)
}

func border(st map[string]string) string {
	width := st["border-top-width"]
	if width == "0px" || st["border-top-style"] == "none" {
		width = "none"
	}
	text := width
	if width != "none" {
		text = fmt.Sprintf("%s %s %s", width, st["border-top-style"], ColorHex(st["border-top-color"]))
	}
	sides := []string{st["border-right-width"], st["border-bottom-width"], st["border-left-width"]}
	for _, s := range sides {
		if s != st["border-top-width"] {
			return text + fmt.Sprintf(" (top; right/bottom/left widths: %s)", strings.Join(sides, " "))
		}
	}
	return text
}

func attrList(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		parts = append(parts, fmt.Sprintf("%s=%s", k, strconv.Quote(Truncate(m[k], 80))))
	}
	return strings.Join(parts, " ")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
