// Command cursorlab gera uma página HTML local para revisar e ajustar o movimento do cursor visual
// (docs/CURSOR.md, seção 10): trajetórias sorteadas por cenário, velocidade × tempo com o pico
// marcado, estatísticas e um palco que toca os planos com o mesmo cursor.js do overlay.
//
//	go run ./tools/cursorlab [-speed normal] [-seed 1] [-n 50] [-set FittsB=120 -set PeakMax=0.42]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/JonathanSantos/vitral/internal/brand"
	"github.com/JonathanSantos/vitral/internal/cursor"
	"github.com/JonathanSantos/vitral/internal/overlay"
)

// O palco tem o tamanho de uma viewport pequena; os cenários usam estas coordenadas.
const stageW, stageH = 960, 540

type scenario struct {
	Name   string
	Note   string
	From   cursor.Point
	Target cursor.Rect
	Kind   cursor.Kind
}

var scenarios = []scenario{
	{"Perto, alvo grande", "botão de 200×60 a ~240 px", cursor.Point{X: 360, Y: 300}, cursor.Rect{X: 520, Y: 250, W: 200, H: 60}, cursor.Pointer},
	{"Longe, alvo pequeno", "checkbox de 14×14 a ~1000 px: submovimentos", cursor.Point{X: 40, Y: 40}, cursor.Rect{X: 893, Y: 473, W: 14, H: 14}, cursor.Pointer},
	{"Diagonal", "botão de 120×36 a ~830 px", cursor.Point{X: 60, Y: 480}, cursor.Rect{X: 760, Y: 60, W: 120, H: 36}, cursor.Pointer},
	{"Vertical", "botão de 160×40 a ~430 px", cursor.Point{X: 480, Y: 40}, cursor.Rect{X: 400, Y: 450, W: 160, H: 40}, cursor.Pointer},
	{"Campo de texto", "terço esquerdo e meio vertical", cursor.Point{X: 820, Y: 440}, cursor.Rect{X: 60, Y: 120, W: 320, H: 40}, cursor.Text},
}

// Alvos da sequência: o cursor sai de onde parou, como numa sessão de verdade.
var sequence = []struct {
	Target cursor.Rect
	Kind   cursor.Kind
}{
	{cursor.Rect{X: 60, Y: 60, W: 320, H: 40}, cursor.Text},
	{cursor.Rect{X: 60, Y: 130, W: 320, H: 40}, cursor.Text},
	{cursor.Rect{X: 60, Y: 200, W: 16, H: 16}, cursor.Pointer},
	{cursor.Rect{X: 420, Y: 60, W: 140, H: 36}, cursor.Pointer},
	{cursor.Rect{X: 800, Y: 470, W: 110, H: 40}, cursor.Pointer},
	{cursor.Rect{X: 600, Y: 300, W: 24, H: 24}, cursor.Pointer},
	{cursor.Rect{X: 120, Y: 440, W: 200, H: 50}, cursor.Pointer},
	{cursor.Rect{X: 880, Y: 30, W: 40, H: 40}, cursor.Pointer},
}

type sets []string

func (s *sets) String() string     { return strings.Join(*s, ",") }
func (s *sets) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	speed := flag.String("speed", "normal", "slow, normal, fast or instant")
	seed := flag.Uint64("seed", 1, "random seed")
	n := flag.Int("n", 50, "trajectories per scenario")
	out := flag.String("out", filepath.Join(".quality", "out", "cursorlab", "index.html"), "output HTML")
	var overrides sets
	flag.Var(&overrides, "set", "override a Profile field, e.g. -set FittsB=120 (repeatable)")
	flag.Parse()

	p := cursor.DefaultProfile()
	sp, err := cursor.ParseSpeed(*speed)
	if err != nil {
		fail(err)
	}
	p.Speed = sp
	if err := apply(&p, overrides); err != nil {
		fail(err)
	}
	page := render(p, *seed, *n, overrides)
	if err := os.MkdirAll(filepath.Dir(*out), 0o750); err != nil {
		fail(err)
	}
	if err := os.WriteFile(*out, []byte(page), 0o600); err != nil {
		fail(err)
	}
	abs, _ := filepath.Abs(*out)
	fmt.Println("cursorlab:", abs)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "cursorlab:", err)
	os.Exit(1)
}

// apply muda campos do Profile por nome (float ou int).
func apply(p *cursor.Profile, overrides []string) error {
	v := reflect.ValueOf(p).Elem()
	for _, o := range overrides {
		name, val, ok := strings.Cut(o, "=")
		if !ok {
			return fmt.Errorf("-set %q: use Field=value", o)
		}
		f := v.FieldByName(name)
		if !f.IsValid() || !f.CanSet() {
			return fmt.Errorf("-set: unknown Profile field %q", name)
		}
		switch f.Kind() {
		case reflect.Float64:
			x, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return fmt.Errorf("-set %s: %w", name, err)
			}
			f.SetFloat(x)
		case reflect.Int:
			x, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("-set %s: %w", name, err)
			}
			f.SetInt(int64(x))
		default:
			return fmt.Errorf("-set: field %q is not a number", name)
		}
	}
	return nil
}

type run struct {
	Plan   cursor.Plan
	From   cursor.Point
	Target cursor.Rect
	Ring   string
}

// runJSON é o que o palco precisa para tocar: plano no formato do overlay mais o contexto.
func (r run) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"plan": r.Plan, "from": []float64{r.From.X, r.From.Y},
		"target": []float64{r.Target.X, r.Target.Y, r.Target.W, r.Target.H}, "ring": r.Ring,
	})
}

func ringColor(i int) string {
	fill, _ := brand.MarkColor(i + 1)
	return brand.Hex(fill)
}

func plans(p cursor.Profile, seed uint64, n int) ([][]run, []run) {
	rng := rand.New(rand.NewPCG(seed, 0xc0ffee))
	all := make([][]run, len(scenarios))
	for si, sc := range scenarios {
		side := 0
		for i := 0; i < n; i++ {
			to := pick(sc.Target, sc.Kind, p, rng)
			plan := cursor.Compute(cursor.Input{From: sc.From, To: to, Target: sc.Target, Kind: sc.Kind, Side: side}, p, rng)
			side = plan.Side
			all[si] = append(all[si], run{plan, sc.From, sc.Target, ringColor(i)})
		}
	}
	var seq []run
	from, side := cursor.Point{X: stageW / 2, Y: stageH / 2}, 0
	for i, s := range sequence {
		to := pick(s.Target, s.Kind, p, rng)
		plan := cursor.Compute(cursor.Input{From: from, To: to, Target: s.Target, Kind: s.Kind, Side: side}, p, rng)
		seq = append(seq, run{plan, from, s.Target, ringColor(i)})
		from, side = plan.End, plan.Side
	}
	return all, seq
}

// pick usa o primeiro candidato (no laboratório não há página para validar o ponto).
func pick(t cursor.Rect, k cursor.Kind, p cursor.Profile, rng *rand.Rand) cursor.Point {
	return cursor.Candidates(t, k, p, rng)[0]
}

// velocity devolve (t no meio do intervalo, px/ms) entre keyframes.
func velocity(pl cursor.Plan) [][2]float64 {
	f := pl.Frames
	var out [][2]float64
	for i := 3; i < len(f); i += 3 {
		dt := f[i] - f[i-3]
		if dt <= 0 {
			continue
		}
		d := math.Hypot(f[i+1]-f[i-2], f[i+2]-f[i-1])
		out = append(out, [2]float64{(f[i] + f[i-3]) / 2, d / dt})
	}
	return out
}

// peakFrac é onde fica o pico de velocidade dentro do movimento principal (0 a 1).
func peakFrac(pl cursor.Plan) (t, v float64) {
	for _, s := range velocity(pl) {
		if s[0] <= pl.MainEnd && s[1] > v {
			t, v = s[0], s[1]
		}
	}
	return t, v
}

func maxDeviation(r run) float64 {
	f := r.Plan.Frames
	dx, dy := r.Plan.End.X-r.From.X, r.Plan.End.Y-r.From.Y
	d := math.Hypot(dx, dy)
	if d == 0 {
		return 0
	}
	var worst float64
	for i := 0; i < len(f); i += 3 {
		worst = math.Max(worst, math.Abs((f[i+1]-r.From.X)*dy-(f[i+2]-r.From.Y)*dx)/d)
	}
	return worst
}

type stats struct{ min, max, sum float64 }

func (s *stats) add(v float64, first bool) {
	if first {
		s.min, s.max = v, v
	}
	s.min, s.max, s.sum = math.Min(s.min, v), math.Max(s.max, v), s.sum+v
}

func render(p cursor.Profile, seed uint64, n int, overrides []string) string {
	all, seq := plans(p, seed, n)
	var b strings.Builder
	data, _ := json.Marshal(map[string]any{"scenarios": all, "sequence": seq})

	fmt.Fprintf(&b, `<!doctype html><html lang="pt-BR"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1"><title>Vitral cursorlab</title>
<style>:root{%s}
body{margin:0;font:14px/1.45 system-ui,sans-serif;background:var(--vitral-paper);color:var(--vitral-ink)}
main{max-width:1000px;margin:0 auto;padding:24px 20px 60px}
h1{font-size:22px;margin:0 0 4px}h2{font-size:17px;margin:28px 0 4px}
.muted{color:var(--vitral-muted)}code{font:12px ui-monospace,monospace}
.card{background:#fff;border:1px solid #e3dfd6;border-radius:12px;padding:16px;margin-top:12px}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:12px}@media(max-width:820px){.grid{grid-template-columns:1fr}}
svg.chart{width:100%%;height:auto;background:#fff;border:1px solid #eee;border-radius:8px}
table{border-collapse:collapse;font-size:13px}td{padding:2px 12px 2px 0}td:first-child{color:var(--vitral-muted)}
.stage{position:relative;width:%dpx;height:%dpx;background:#fff;border:1px solid #e3dfd6;border-radius:8px;overflow:hidden;transform-origin:0 0}
.stage-wrap{overflow:hidden}
.t{position:absolute;border:1.5px dashed var(--vitral-muted);border-radius:6px;background:rgba(62,99,221,.06)}
.t.text{background:#fafafa;border-style:solid}
.layer{position:absolute;inset:0}
button{font:600 13px system-ui,sans-serif;padding:7px 14px;border-radius:999px;border:0;background:var(--vitral-ink);color:var(--vitral-paper);cursor:pointer}
button.sec{background:#e9e5dc;color:var(--vitral-ink)}
.controls{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin:10px 0}
select{font:13px system-ui;padding:5px}
</style></head><body><main>
<h1>Vitral cursorlab</h1>
<p class="muted">Velocidade <b>%s</b> · semente <code>%d</code> · %d trajetórias por cenário%s.
Regere com <code>go run ./tools/cursorlab -speed slow -seed 2 -set FittsB=120</code>.</p>
<div class="controls"><button class="tab" data-tab="cursor">Cursor</button><button class="tab sec" data-tab="scroll">Scroll</button></div>
<div id="tab-cursor">`,
		brand.CSSVars(), stageW, stageH, p.Speed, seed, n, overrideNote(overrides))

	fmt.Fprint(&b, `<div class="card"><h2 style="margin-top:0">Palco</h2>
<p class="muted">Toca os planos em tempo real com o mesmo <code>cursor.js</code> do overlay. A sequência sai de onde parou, como numa sessão.</p>
<div class="controls"><button id="play-seq">▶ Tocar sequência</button>
<select id="sc">`)
	for i, sc := range scenarios {
		fmt.Fprintf(&b, `<option value="%d">%s</option>`, i, html.EscapeString(sc.Name))
	}
	fmt.Fprintf(&b, `</select><select id="idx">`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<option value="%d">#%d</option>`, i, i+1)
	}
	fmt.Fprint(&b, `</select><button id="play-one" class="sec">▶ Tocar esta</button><button id="play-five" class="sec">▶ Tocar 5 seguidas</button>
<span id="info" class="muted"></span></div>
<div class="stage-wrap" id="wrap"><div class="stage" id="stage"><div class="layer" id="targets"></div><div class="layer" id="layer"></div></div></div></div>`)

	for si, sc := range scenarios {
		writeScenario(&b, p, sc, all[si])
	}
	b.WriteString(`</div><div id="tab-scroll" hidden>`)
	scrollData := writeScrollTab(&b, p.Speed, seed)
	b.WriteString(`</div>`)

	fmt.Fprintf(&b, `</main><script>%s
%s
const DATA = %s;
const SCROLL = %s;
%s
%s</script></body></html>`, overlay.CursorScript, overlay.ScrollScript, data, scrollData, labJS, scrollJS)
	return b.String()
}

func overrideNote(o []string) string {
	if len(o) == 0 {
		return ""
	}
	return " · ajustes <code>" + html.EscapeString(strings.Join(o, " ")) + "</code>"
}

func writeScenario(b *strings.Builder, p cursor.Profile, sc scenario, runs []run) {
	var dur, peak, dev stats
	var subs, overs int
	for i, r := range runs {
		dur.add(r.Plan.Move, i == 0)
		pt, _ := peakFrac(r.Plan)
		frac := 0.0
		if r.Plan.MainEnd > 0 {
			frac = pt / r.Plan.MainEnd
		}
		peak.add(frac, i == 0)
		dev.add(maxDeviation(r), i == 0)
		if r.Plan.Submove {
			subs++
		}
		if r.Plan.Overshoot > 0 {
			overs++
		}
	}
	d := math.Hypot(sc.Target.Center().X-sc.From.X, sc.Target.Center().Y-sc.From.Y)
	w := math.Max(math.Min(sc.Target.W, sc.Target.H), p.MinWidth)
	id := math.Log2(d/w + 1)
	nRuns := float64(len(runs))
	fmt.Fprintf(b, `<h2>%s</h2><p class="muted">%s · D ≈ %.0f px · W = %.0f px · ID ≈ %.2f bits</p><div class="grid">`,
		html.EscapeString(sc.Name), html.EscapeString(sc.Note), d, w, id)
	b.WriteString(trajectories(sc, runs))
	b.WriteString(velocities(runs))
	fmt.Fprintf(b, `</div><div class="card"><table>
<tr><td>duração do movimento</td><td>%.0f–%.0f ms (média %.0f)</td></tr>
<tr><td>pico de velocidade</td><td>%.0f%%–%.0f%% da duração principal (alvo: %.0f%%–%.0f%%)</td></tr>
<tr><td>desvio máximo da reta</td><td>%.1f–%.1f px (limite: %.1f px)</td></tr>
<tr><td>com submovimento</td><td>%.0f%% (overshoot em %.0f%% deles)</td></tr>
</table></div>`,
		dur.min, dur.max, dur.sum/nRuns, 100*peak.min, 100*peak.max, 100*p.PeakMin, 100*p.PeakMax,
		dev.min, dev.max, math.Min(p.ArcLimit*d, p.ArcLimitPx), 100*float64(subs)/nRuns, pct(overs, subs))
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

// trajectories desenha os caminhos sobre o palco (mesma escala), com início, alvo e pontos finais.
func trajectories(sc scenario, runs []run) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %d %d" role="img" aria-label="trajetórias">`, stageW, stageH)
	t := sc.Target
	fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="6" fill="rgba(62,99,221,.08)" stroke="#5b5770" stroke-dasharray="4 3"/>`, t.X, t.Y, t.W, t.H)
	in := t.Inset(0.7)
	fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="none" stroke="#5b5770" stroke-width=".6" opacity=".5"/>`, in.X, in.Y, in.W, in.H)
	for i, r := range runs {
		var pts []string
		f := r.Plan.Frames
		for j := 0; j < len(f); j += 3 {
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", f[j+1], f[j+2]))
		}
		fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="1.2" opacity=".55"/>`, strings.Join(pts, " "), ringColor(i))
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="2" fill="%s"/>`, r.Plan.End.X, r.Plan.End.Y, ringColor(i))
	}
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="5" fill="#1e1b2e"/></svg>`, sc.From.X, sc.From.Y)
	return b.String()
}

// velocities desenha velocidade × tempo de cada trajetória, com o pico marcado.
func velocities(runs []run) string {
	const w, h, pad = 460, 270, 34
	var maxT, maxV float64
	for _, r := range runs {
		maxT = math.Max(maxT, r.Plan.Move)
		for _, s := range velocity(r.Plan) {
			maxV = math.Max(maxV, s[1])
		}
	}
	maxT, maxV = math.Max(maxT, 1), math.Max(maxV, 0.01)
	x := func(t float64) float64 { return pad + (w-pad-8)*t/maxT }
	y := func(v float64) float64 { return h - pad + 6 - (h-pad-6)*v/maxV }
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %d %d" role="img" aria-label="velocidade por tempo">`, w, h)
	fmt.Fprintf(&b, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#ccc"/><line x1="%d" y1="6" x2="%d" y2="%.1f" stroke="#ccc"/>`,
		pad, y(0), w-8, y(0), pad, pad, y(0))
	for _, tick := range ticks(maxT) {
		fmt.Fprintf(&b, `<text x="%.1f" y="%d" font-size="10" text-anchor="middle" fill="#5b5770">%.0f</text>`, x(tick), h-12, tick)
	}
	fmt.Fprintf(&b, `<text x="%d" y="%d" font-size="10" text-anchor="end" fill="#5b5770">ms</text>`, w-8, h-2)
	fmt.Fprintf(&b, `<text x="4" y="14" font-size="10" fill="#5b5770">%.1f px/ms</text>`, maxV)
	for i, r := range runs {
		var pts []string
		for _, s := range velocity(r.Plan) {
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", x(s[0]), y(s[1])))
		}
		fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="1" opacity=".45"/>`, strings.Join(pts, " "), ringColor(i))
		pt, pv := peakFrac(r.Plan)
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="2.4" fill="%s" stroke="#fff" stroke-width=".6"/>`, x(pt), y(pv), ringColor(i))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func ticks(maxT float64) []float64 {
	step := 100.0
	if maxT > 1000 {
		step = 250
	}
	var out []float64
	for t := 0.0; t <= maxT; t += step {
		out = append(out, t)
	}
	sort.Float64s(out)
	return out
}

const labJS = `
const css = document.createElement('style');
css.textContent = VITRAL_CURSOR_CSS;
document.head.append(css);
const stage = document.getElementById('stage');
const wrap = document.getElementById('wrap');
const targets = document.getElementById('targets');
const info = document.getElementById('info');
const fit = () => {
  const k = Math.min(1, wrap.clientWidth / stage.offsetWidth);
  stage.style.transform = 'scale(' + k + ')';
  wrap.style.height = (stage.offsetHeight * k) + 'px';
};
addEventListener('resize', fit); fit();
const cur = vitralCursor(document.getElementById('layer'), 'IA');
const drawTargets = (runs) => {
  targets.innerHTML = '';
  const seen = new Set();
  for (const r of runs) {
    const key = r.target.join(',');
    if (seen.has(key)) continue;
    seen.add(key);
    const d = document.createElement('div');
    d.className = 't' + (r.plan.shape === 'text' ? ' text' : '');
    Object.assign(d.style, { left: r.target[0] + 'px', top: r.target[1] + 'px', width: r.target[2] + 'px', height: r.target[3] + 'px' });
    targets.append(d);
  }
};
let token = 0;
const playAll = async (runs, fromEach) => {
  const mine = ++token;
  drawTargets(runs);
  for (let i = 0; i < runs.length && mine === token; i++) {
    const r = runs[i];
    if (fromEach || i === 0) cur.place(r.from[0], r.from[1]);
    info.textContent = 'movimento ' + Math.round(r.plan.move) + ' ms · press em ' + Math.round(r.plan.press) + ' ms';
    await new Promise((ok) => cur.play(r.plan, { ring: r.ring }, ok));
    if (fromEach) await new Promise((ok) => setTimeout(ok, 500));
  }
};
document.getElementById('play-seq').onclick = () => playAll(DATA.sequence, false);
const pickRuns = (count) => {
  const s = +document.getElementById('sc').value, i = +document.getElementById('idx').value;
  return DATA.scenarios[s].slice(i, i + count);
};
document.getElementById('play-one').onclick = () => playAll(pickRuns(1), true);
document.getElementById('play-five').onclick = () => playAll(pickRuns(5), true);
`
