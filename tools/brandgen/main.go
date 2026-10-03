// Comando brandgen gera os PNGs da marca (ícones e logotipo) a partir de assets/brand/*.svg,
// renderizando no Chromium via rod. Uso: go run ./tools/brandgen (ou make brand).
//
// É uma ferramenta de desenvolvimento: o binário vitral não depende dela. A fonte do logotipo
// (Space Grotesk, OFL) é carregada de tools/brandgen/fonts, então a geração funciona offline
// e sempre dá o mesmo resultado.
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/brand"
)

// Tamanhos da seção 3.2 do BRAND.md: o ícone sem selo vai até 48 px.
var iconSizes = []int{16, 32, 48, 128, 256, 512}

const smallMax = 48

func main() {
	dir := flag.String("dir", "assets/brand", "folder with the SVGs; PNGs are written here")
	fonts := flag.String("fonts", "tools/brandgen/fonts", "folder with SpaceGrotesk-latin.woff2")
	flag.Parse()
	if err := run(*dir, *fonts); err != nil {
		log.Fatal(err)
	}
}

func run(dir, fontsDir string) error {
	font, err := os.ReadFile(filepath.Join(fontsDir, "SpaceGrotesk-latin.woff2"))
	if err != nil {
		return fmt.Errorf("read font: %w", err)
	}
	fontFace := fmt.Sprintf(`@font-face{font-family:'Space Grotesk';font-weight:300 700;src:url(data:font/woff2;base64,%s) format('woff2')}`,
		base64.StdEncoding.EncodeToString(font))

	svgs := map[string]string{}
	for _, name := range []string{"icon", "icon-dark", "icon-small"} {
		data, err := os.ReadFile(filepath.Join(dir, name+".svg"))
		if err != nil {
			return err
		}
		svgs[name] = string(data)
	}

	bin, ok := launcher.LookPath()
	if !ok {
		return fmt.Errorf("Chrome/Chromium not found")
	}
	u, err := launcher.New().Bin(bin).Headless(true).Launch()
	if err != nil {
		return err
	}
	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		return err
	}
	defer b.Close()
	p, err := b.Page(proto.TargetCreateTarget{})
	if err != nil {
		return err
	}
	transparent := proto.EmulationSetDefaultBackgroundColorOverride{Color: &proto.DOMRGBA{A: new(0.0)}}
	if err := transparent.Call(p); err != nil {
		return err
	}

	for _, size := range iconSizes {
		svg := svgs["icon"]
		if size <= smallMax {
			svg = svgs["icon-small"]
		}
		body := fmt.Sprintf(`<div id="shot" style="width:%dpx;height:%dpx">%s</div>`, size, size, sized(svg))
		data, err := render(p, fontFace, body, 1)
		if err != nil {
			return err
		}
		if err := write(dir, fmt.Sprintf("icon-%d.png", size), data); err != nil {
			return err
		}
		if size == 512 {
			if err := write(dir, "icon.png", data); err != nil {
				return err
			}
		}
	}

	// Logotipo: ícone + "vitral" (seção 4). Ícone ≈ 0,9× a altura da palavra, espaço ≈ 0,2× o ícone.
	for _, variant := range []struct{ file, icon, text string }{
		{"logo.png", "icon", brand.Hex(brand.Ink)},
		{"logo-dark.png", "icon-dark", brand.Hex(brand.Paper)},
	} {
		body := fmt.Sprintf(`<div id="shot" style="display:inline-flex;align-items:center;gap:18px;padding:12px">
<div style="width:88px;height:88px">%s</div>
<span style="font-family:'Space Grotesk';font-weight:700;font-size:96px;letter-spacing:-0.04em;line-height:1;color:%s">vitral</span>
</div>`, sized(svgs[variant.icon]), variant.text)
		data, err := render(p, fontFace, body, 2)
		if err != nil {
			return err
		}
		if err := write(dir, variant.file, data); err != nil {
			return err
		}
	}
	return nil
}

// sized faz o SVG ocupar o contêiner.
func sized(svg string) string {
	return strings.Replace(svg, "<svg ", `<svg width="100%" height="100%" `, 1)
}

func render(p *rod.Page, fontFace, body string, scale float64) ([]byte, error) {
	viewport := proto.EmulationSetDeviceMetricsOverride{Width: 1200, Height: 800, DeviceScaleFactor: 1}
	if err := viewport.Call(p); err != nil {
		return nil, err
	}
	html := fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><style>%s html,body{margin:0;background:transparent}</style></head><body>%s</body></html>`, fontFace, body)
	if err := p.SetDocumentContent(html); err != nil {
		return nil, err
	}
	// Garante que a Space Grotesk carregou antes da captura (o "1" do selo e o logotipo usam a fonte).
	if _, err := p.Eval(`() => document.fonts.load('700 34px "Space Grotesk"').then(() => document.fonts.ready)`); err != nil {
		return nil, err
	}
	var box struct{ X, Y, W, H float64 }
	res, err := p.Eval(`() => { const r = document.getElementById('shot').getBoundingClientRect(); return {X: r.x, Y: r.y, W: r.width, H: r.height}; }`)
	if err != nil {
		return nil, err
	}
	if err := res.Value.Unmarshal(&box); err != nil {
		return nil, err
	}
	shot, err := proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
		Clip:   &proto.PageViewport{X: box.X, Y: box.Y, Width: box.W, Height: box.H, Scale: scale},
	}.Call(p)
	if err != nil {
		return nil, err
	}
	return shot.Data, nil
}

func write(dir, name string, data []byte) error {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	return nil
}
