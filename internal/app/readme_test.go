//go:build integration && readme

package app_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/cursor"
	"github.com/JonathanSantos/vitral/internal/ux"
)

// Prints do README (docs/img), tirados do navegador de verdade com o overlay: make readme-shots.
// Não é regressão (não compara nada); só regrava as imagens. A IA nunca vê o overlay: o screenshot
// da ferramenta o esconde, por isso aqui a captura é direta pelo CDP.
func TestReadmeShots(t *testing.T) {
	p, err := h.app.Browser.Page()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join("..", "..", "docs", "img")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	save := func(name string, img image.Image) {
		t.Helper()
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	ov := h.app.Overlay
	// Janela menor que a dos testes: o selo fica perto do conteúdo e o print não sobra vazio.
	if err := p.SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: 960, Height: 420, DeviceScaleFactor: 1}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proto.EmulationClearDeviceMetricsOverride{}.Call(p) }()
	vw, vh := viewport(t, p)
	full := image.Rect(0, 0, vw, vh)

	// 1. O que a pessoa vê: selo "trabalhando", destaque com a ref e o cursor "IA" indo ao link.
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/mini/pedidos.html")})
	details := ref(t, snap, "link", "Ver detalhes")
	box := boxOf(t, h.ok(t, "inspect", map[string]any{"ref": details}))
	ov.ToolStarted()
	ov.ShowHighlight(ctx, details, [4]int{box.Min.X, box.Min.Y, box.Dx(), box.Dy()}, time.Minute)
	at := cursor.Point{X: float64(box.Min.X + box.Dx()/2), Y: float64(box.Min.Y + box.Dy()/2)}
	if err := ov.PlaceCursor(ctx, cursor.Point{X: 420, Y: 420}, true); err != nil {
		t.Fatal(err)
	}
	prof := cursor.DefaultProfile()
	rng := rand.New(rand.NewPCG(7, 8))
	ov.PlayCursor(ctx, cursor.Compute(cursor.Input{From: at, To: at, Target: cursor.Rect{X: at.X - 10, Y: at.Y - 10, W: 20, H: 20}, Kind: cursor.Pointer}, prof, rng), "", false)
	save("tela-ia-trabalhando", stable(t, p, full))
	ov.ToolFinished()
	_ = ov.HideCursor(ctx)

	// 2. O que a IA vê: o screenshot da ferramenta, com as marcações por ref.
	img, _ := h.shot(t, map[string]any{})
	save("visao-da-ia", img)

	// 3. Os estados do selo, empilhados.
	badge := image.Rect(vw-620, vh-110, vw, vh)
	var badges []image.Image
	for _, s := range []struct{ events, reset []ux.Event }{
		{events: []ux.Event{{Kind: ux.ToolStarted}}},
		{events: []ux.Event{{Kind: ux.FlowStep, FlowName: "pedido-semanal", Step: 2, Total: 4}}, reset: []ux.Event{{Kind: ux.ToolFinished}}},
		{events: []ux.Event{{Kind: ux.UserPause}}, reset: []ux.Event{{Kind: ux.UserResume}}},
	} {
		for _, e := range s.events {
			ov.Apply(e)
		}
		badges = append(badges, stable(t, p, badge))
		for _, e := range s.reset {
			ov.Apply(e)
		}
	}
	save("selo-estados", stack(badges))

	// 4. "Sua vez" num campo de código (2FA): a IA nunca digita ali.
	h.call(t, "open", map[string]any{"url": h.url("/pages/verify/codigo.html")}) // sem janela: "verification required"
	snap = h.ok(t, "snapshot", nil)
	code := ref(t, snap, "textbox", "Código")
	box = boxOf(t, h.ok(t, "inspect", map[string]any{"ref": code}))
	ov.WaitForUser(ux.ReasonCode, code, [4]int{box.Min.X, box.Min.Y, box.Dx(), box.Dy()})
	save("sua-vez", stable(t, p, full))
	ov.Apply(ux.Event{Kind: ux.UserResume})

	// 5. O cursorlab (make cursorlab antes): trajetórias e o perfil da rolagem.
	lab := filepath.Join("..", "..", ".quality", "out", "cursorlab", "index.html")
	abs, err := filepath.Abs(lab)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Skip("run make cursorlab first")
	}
	lp, err := p.Browser().Page(proto.TargetCreateTarget{URL: "file://" + abs})
	if err != nil {
		t.Fatal(err)
	}
	defer lp.Close()
	_ = lp.SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: 1400, Height: 900, DeviceScaleFactor: 1})
	_ = lp.WaitLoad()
	lp = lp.Timeout(time.Minute)
	for _, sec := range []struct{ tab, title, name string }{
		{"cursor", "Longe, alvo pequeno", "cursor-trajetorias"},
		{"scroll", "Uma tela", "rolagem-perfil"},
	} {
		// Clique pelo DOM: o palco anima e o clique do rod esperaria o elemento parar.
		if _, err := lp.Eval(`(id) => document.querySelector('button[data-tab="' + id + '"]').click()`, sec.tab); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		// A seção vai do título visível até o próximo título visível (coordenadas do documento).
		res, err := lp.Eval(`(title) => {
			const hs = [...document.querySelectorAll('h2')].filter(h => h.checkVisibility());
			const i = hs.findIndex(h => h.textContent === title);
			const top = hs[i].getBoundingClientRect().top + scrollY - 12;
			const end = hs[i + 1].getBoundingClientRect().top + scrollY - 24;
			const box = hs[i].parentElement.getBoundingClientRect();
			return [box.left - 12, top, box.width + 24, end - top];
		}`, sec.title)
		if err != nil {
			t.Fatal(err)
		}
		r := res.Value.Arr()
		data, err := lp.Screenshot(true, &proto.PageCaptureScreenshot{
			Format: proto.PageCaptureScreenshotFormatPng,
			Clip:   &proto.PageViewport{X: r[0].Num(), Y: r[1].Num(), Width: r[2].Num(), Height: r[3].Num(), Scale: 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, sec.name+".png"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// stack empilha imagens de mesma largura com um respiro claro entre elas.
func stack(imgs []image.Image) image.Image {
	const gap = 0
	w, hgt := 0, 0
	for _, im := range imgs {
		w = max(w, im.Bounds().Dx())
		hgt += im.Bounds().Dy() + gap
	}
	out := image.NewRGBA(image.Rect(0, 0, w, hgt))
	draw.Draw(out, out.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	y := 0
	for _, im := range imgs {
		draw.Draw(out, image.Rect(0, y, w, y+im.Bounds().Dy()), im, im.Bounds().Min, draw.Src)
		y += im.Bounds().Dy() + gap
	}
	return out
}

// GIFs do README (docs/img/*.gif): as ações de verdade, com o cursor ligado sem janela (force), gravadas
// pelo screencast do CDP. Cada GIF guarda só a área que mudou entre os quadros.
func TestReadmeGIFs(t *testing.T) {
	hh, cleanup := newHarness(func(c *config.Config) {
		c.Cursor, c.CursorSpeed, c.CursorSeed, c.HighlightMs, c.TypePacing = "force", "normal", 3, 350, "on"
	})
	defer cleanup()
	hh.ok(t, "open", map[string]any{"url": hh.url("/pages/mini/index.html")})
	p := hh.page(t)
	if err := p.SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: 960, Height: 540, DeviceScaleFactor: 1}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join("..", "..", "docs", "img")
	// Cada ação lê a tela antes (o snapshot não mexe em nada visível).
	// Entre uma ação e outra a IA lê e pensa: sem essa pausa o destaque da ação anterior ainda está
	// sumindo quando a próxima rolagem começa, o que nunca acontece numa conversa de verdade.
	think := func() { time.Sleep(700 * time.Millisecond) }
	// Alvo fora da área visível não está no snapshot: aí lê a página inteira.
	click := func(role, name string) {
		defer think()
		snap := hh.ok(t, "snapshot", nil)
		if !strings.Contains(snap, role+` "`+name+`"`) {
			snap = hh.ok(t, "snapshot", map[string]any{"full": true})
		}
		hh.ok(t, "click", map[string]any{"ref": ref(t, snap, role, name)})
	}
	typeIn := func(role, name, text string) {
		defer think()
		hh.ok(t, "type", map[string]any{"ref": ref(t, hh.ok(t, "snapshot", nil), role, name), "text": text})
	}

	// A IA usando o mini ERP: menu, detalhe, exportar e uma busca digitada.
	writeGIF(t, filepath.Join(out, "ia-na-tela.gif"), record(t, p, func() {
		click("link", "Pedidos")
		typeIn("searchbox", "Buscar pedido", "10234")
		click("link", "Ver detalhes")
		click("button", "Exportar CSV")
		click("link", "Clientes")
	}))

	// O cursor: alvo grande, caixinha pequena, link no meio do texto, campo, card com ação dentro.
	hh.ok(t, "open", map[string]any{"url": hh.url("/pages/cursor-playground.html")})
	writeGIF(t, filepath.Join(out, "cursor.gif"), record(t, p, func() {
		click("button", "Continuar")
		click("checkbox", "Aceito os termos")
		click("link", "política de privacidade")
		typeIn("textbox", "Mensagem", "Olá! Aqui é o Vitral.")
		click("button", "Pedido 1042")
	}))

	// A rolagem: até um alvo a três telas, depois bem longe, e de volta ao topo.
	hh.ok(t, "open", map[string]any{"url": hh.url("/pages/scroll-playground.html")})
	writeGIF(t, filepath.Join(out, "rolagem.gif"), record(t, p, func() {
		click("button", "Alvo a três telas")
		click("button", "Alvo bem longe")
		click("button", "Alvo perto")
	}))
}

type frame struct {
	img image.Image
	at  float64 // segundos (timestamp do screencast)
}

// record grava os quadros que o compositor produz enquanto fn roda.
func record(t *testing.T, p *rod.Page, fn func()) []frame {
	t.Helper()
	var (
		mu     sync.Mutex
		frames []frame
	)
	ctx, cancel := context.WithCancel(context.Background())
	wait := p.Context(ctx).EachEvent(func(e *proto.PageScreencastFrame) {
		_ = proto.PageScreencastFrameAck{SessionID: e.SessionID}.Call(p)
		img, err := png.Decode(bytes.NewReader(e.Data))
		if err != nil {
			return
		}
		mu.Lock()
		frames = append(frames, frame{img: img, at: float64(e.Metadata.Timestamp)})
		mu.Unlock()
	})
	done := make(chan struct{})
	go func() { wait(); close(done) }()
	if err := (proto.PageStartScreencast{Format: proto.PageStartScreencastFormatPng}).Call(p); err != nil {
		t.Fatal(err)
	}
	time.Sleep(600 * time.Millisecond)
	fn()
	time.Sleep(1200 * time.Millisecond)
	_ = proto.PageStopScreencast{}.Call(p)
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if len(frames) < 2 {
		t.Fatalf("screencast gave %d frames", len(frames))
	}
	return frames
}

// writeGIF monta o GIF com uma paleta das cores mais usadas (telas de sistema têm poucas cores) e
// guarda em cada quadro só o retângulo que mudou.
func writeGIF(t *testing.T, path string, frames []frame) {
	t.Helper()
	count := map[color.RGBA]int{}
	for i, f := range frames {
		if i%4 != 0 {
			continue
		}
		b := f.img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y += 2 {
			for x := b.Min.X; x < b.Max.X; x += 2 {
				r, g, bb, _ := f.img.At(x, y).RGBA()
				count[color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bb >> 8), 255}]++
			}
		}
	}
	type kv struct {
		c color.RGBA
		n int
	}
	var all []kv
	for c, n := range count {
		all = append(all, kv{c, n})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n })
	pal := color.Palette{}
	for _, e := range all {
		if len(pal) == 256 {
			break
		}
		pal = append(pal, e.c)
	}
	cache := map[color.RGBA]uint8{}
	toPaletted := func(img image.Image) *image.Paletted {
		b := img.Bounds()
		out := image.NewPaletted(b, pal)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bb, _ := img.At(x, y).RGBA()
				c := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bb >> 8), 255}
				i, ok := cache[c]
				if !ok {
					i = uint8(pal.Index(c))
					cache[c] = i
				}
				out.SetColorIndex(x, y, i)
			}
		}
		return out
	}
	anim := &gif.GIF{}
	var prev *image.Paletted
	for i, f := range frames {
		cur := toPaletted(f.img)
		delay := 150 // o último quadro fica 1,5 s
		if i+1 < len(frames) {
			delay = max(2, int((frames[i+1].at-f.at)*100+0.5))
		}
		if prev == nil {
			anim.Image, anim.Delay, prev = append(anim.Image, cur), append(anim.Delay, delay), cur
			continue
		}
		box := changed(prev, cur)
		if box.Empty() {
			anim.Delay[len(anim.Delay)-1] += delay
			continue
		}
		anim.Image = append(anim.Image, cur.SubImage(box).(*image.Paletted))
		anim.Delay = append(anim.Delay, delay)
		prev = cur
	}
	anim.Delay[len(anim.Delay)-1] = max(anim.Delay[len(anim.Delay)-1], 150)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, anim); err != nil {
		t.Fatal(err)
	}
}

// changed é o menor retângulo com pixels diferentes entre dois quadros.
func changed(a, b *image.Paletted) image.Rectangle {
	r := image.Rectangle{}
	bounds := a.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if a.ColorIndexAt(x, y) != b.ColorIndexAt(x, y) {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}
