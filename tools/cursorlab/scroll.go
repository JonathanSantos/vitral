package main

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"math/rand/v2"
	"strings"

	"github.com/JonathanSantos/vitral/internal/cursor"
)

// Viewport do palco de scroll (a mesma altura do palco do cursor).
const scrollVH = float64(stageH)

var scrollScenarios = []struct {
	Name  string
	Views float64
}{
	{"Meia tela", 0.5}, {"Uma tela", 1.2}, {"Três telas", 3}, {"Cinco telas", 5}, {"Oito telas (salto)", 8}, {"Duas telas para cima", -2},
}

// Modelos para comparar: fim do flick (suave em 2,2τ ou natural até 1%) e teto de px/frame.
var scrollVariants = []struct {
	Name    string
	Natural bool
	MaxStep float64
}{
	{"Padrão: fim suave em 2,2τ (τ 220–320 ms), teto 12% da viewport/frame", false, 0.12},
	{"Fim suave em 2,2τ, sem teto de velocidade", false, 0},
	{"Natural até 1% do pico (τ 140–200 ms), teto 12%", true, 0.12},
	{"Natural até 1% do pico (τ 140–200 ms), sem teto", true, 0},
}

func scrollProfile(v int) cursor.ScrollProfile {
	p := cursor.DefaultScrollProfile()
	p.MaxStep = scrollVariants[v].MaxStep
	if scrollVariants[v].Natural {
		p.Natural, p.TauMin, p.TauMax = true, 140, 200
	}
	return p
}

// writeScrollTab desenha a aba de scroll e devolve os planos (JSON) para o palco.
func writeScrollTab(b *strings.Builder, speed cursor.Speed, seed uint64) string {
	const n = 30
	all := make([][][]cursor.ScrollPlan, len(scrollVariants))
	for v := range scrollVariants {
		p := scrollProfile(v)
		rng := rand.New(rand.NewPCG(seed, 0x5c7011)) // mesma semente em todas: só o modelo muda
		all[v] = make([][]cursor.ScrollPlan, len(scrollScenarios))
		for i, sc := range scrollScenarios {
			for j := 0; j < n; j++ {
				all[v][i] = append(all[v][i], cursor.Scroll(sc.Views*scrollVH, scrollVH, speed, cursor.Animated, p, rng))
			}
		}
	}
	fmt.Fprintf(b, `<div class="card"><h2 style="margin-top:0">Palco do scroll</h2>
<p class="muted">Uma página longa (com header fixo) rolada pelo mesmo <code>scroll.js</code> do overlay, frame a frame com <code>scrollTo(instant)</code>. Viewport de %.0f px.</p>
<div class="controls"><select id="ssc">`, scrollVH)
	for i, sc := range scrollScenarios {
		fmt.Fprintf(b, `<option value="%d">%s</option>`, i, html.EscapeString(sc.Name))
	}
	b.WriteString(`</select><select id="svar">`)
	for v, sv := range scrollVariants {
		fmt.Fprintf(b, `<option value="%d">%s</option>`, v, html.EscapeString(sv.Name))
	}
	b.WriteString(`</select><button id="splay">▶ Tocar</button><button id="sold" class="sec">▶ Como era (passos de roda)</button><span id="sinfo" class="muted"></span></div>
<div class="stage-wrap"><div id="spage" style="width:100%;height:` + fmt.Sprint(stageH) + `px;overflow:auto;border:1px solid #e3dfd6;border-radius:8px;background:#fff;position:relative">
<div style="position:sticky;top:0;height:48px;background:#1e1b2e;color:#f7f5f0;display:flex;align-items:center;padding:0 16px;font-weight:600;z-index:1">Header fixo</div>
<div id="slong"></div></div></div></div>`)
	for v := range scrollVariants {
		hidden := ""
		if v > 0 {
			hidden = " hidden"
		}
		fmt.Fprintf(b, `<div class="svariant" data-v="%d"%s>`, v, hidden)
		for i, sc := range scrollScenarios {
			writeScrollScenario(b, sc.Name, sc.Views, all[v][i], scrollProfile(v))
		}
		b.WriteString(`</div>`)
	}
	data, _ := json.Marshal(all)
	return string(data)
}

func writeScrollScenario(b *strings.Builder, name string, views float64, plans []cursor.ScrollPlan, p cursor.ScrollProfile) {
	var minD, maxD, maxStep, maxPeak, planned float64
	flicks := map[int]int{}
	jumps := 0
	minD = math.Inf(1)
	for _, pl := range plans {
		minD, maxD = math.Min(minD, pl.Duration), math.Max(maxD, pl.Duration)
		flicks[pl.Flicks]++
		planned = math.Max(planned, pl.PeakStep)
		if pl.Jump != 0 {
			jumps++
		}
		f := pl.Frames
		for i := 2; i < len(f); i += 2 {
			if i == 2 && pl.Jump != 0 {
				continue // o salto não é um passo de animação
			}
			maxStep = math.Max(maxStep, math.Abs(f[i+1]-f[i-1]))
		}
		for k := range pl.Starts {
			maxPeak = math.Max(maxPeak, (pl.Peaks[k]-pl.Starts[k])/(pl.Ends[k]-pl.Starts[k]))
		}
	}
	fmt.Fprintf(b, `<h2>%s</h2><p class="muted">%.1f viewports (%.0f px)</p><div class="grid">`, html.EscapeString(name), views, views*scrollVH)
	b.WriteString(scrollChart(plans, false))
	b.WriteString(scrollChart(plans, true))
	var fl []string
	for k := 1; k <= p.MaxFlicks; k++ {
		if flicks[k] > 0 {
			fl = append(fl, fmt.Sprintf("%d flick(s): %d", k, flicks[k]))
		}
	}
	fmt.Fprintf(b, `</div><div class="card"><table>
<tr><td>duração</td><td>%.0f–%.0f ms (teto %.0f ms)</td></tr>
<tr><td>flicks</td><td>%s · com salto: %d de %d</td></tr>
<tr><td>pico de velocidade</td><td><b>%.0f px/frame · %.0f px/s</b> (%.1f%% da viewport por frame%s; antes: 100 px secos a cada ~85 ms, e até 2.537 px num frame indo até um alvo)</td></tr>
<tr><td>maior passo medido nos keyframes</td><td>%.0f px</td></tr>
<tr><td>posição do pico</td><td>até %.0f%% de cada flick (regra: primeiros 30%%)</td></tr>
</table></div>`, minD, maxD, p.Cap, strings.Join(fl, ", "), jumps, len(plans), planned, planned/p.Frame*1000, 100*planned/scrollVH, capNote(p), maxStep, 100*maxPeak)
}

func capNote(p cursor.ScrollProfile) string {
	if p.MaxStep == 0 {
		return ", sem teto"
	}
	return fmt.Sprintf(", teto %.0f%%", 100*p.MaxStep)
}

// scrollChart desenha posição × tempo (ou velocidade × tempo) das trajetórias.
func scrollChart(plans []cursor.ScrollPlan, velocity bool) string {
	const w, h, pad = 460, 240, 34
	var maxT, maxY float64
	series := make([][][2]float64, len(plans))
	for i, pl := range plans {
		f := pl.Frames
		maxT = math.Max(maxT, pl.Duration)
		for j := 0; j < len(f); j += 2 {
			y := math.Abs(f[j+1])
			if velocity {
				if j == 0 || (j == 2 && pl.Jump != 0) {
					continue
				}
				y = math.Abs(f[j+1]-f[j-1]) / (f[j] - f[j-2])
				series[i] = append(series[i], [2]float64{(f[j] + f[j-2]) / 2, y})
			} else {
				series[i] = append(series[i], [2]float64{f[j], y})
			}
			maxY = math.Max(maxY, y)
		}
	}
	maxT, maxY = math.Max(maxT, 1), math.Max(maxY, 0.01)
	x := func(t float64) float64 { return pad + (w-pad-8)*t/maxT }
	y := func(v float64) float64 { return h - pad + 6 - (h-pad-6)*v/maxY }
	label := "posição (px)"
	if velocity {
		label = "velocidade (px/ms)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %d %d" role="img" aria-label="%s">`, w, h, label)
	fmt.Fprintf(&b, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#ccc"/><text x="4" y="14" font-size="10" fill="#5b5770">%s · máx %.1f</text>`, pad, y(0), w-8, y(0), label, maxY)
	fmt.Fprintf(&b, `<text x="%d" y="%d" font-size="10" text-anchor="end" fill="#5b5770">%.0f ms</text>`, w-8, h-4, maxT)
	for i, s := range series {
		var pts []string
		for _, pt := range s {
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", x(pt[0]), y(pt[1])))
		}
		fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="1" opacity=".5"/>`, strings.Join(pts, " "), ringColor(i))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

const scrollJS = `
document.getElementById('svar').onchange = (e) => {
  document.querySelectorAll('.svariant').forEach((d) => { d.hidden = d.dataset.v !== e.target.value; });
};
document.querySelectorAll('.tab').forEach((btn) => btn.onclick = () => {
  document.querySelectorAll('.tab').forEach((x) => x.classList.toggle('sec', x !== btn));
  document.getElementById('tab-cursor').hidden = btn.dataset.tab !== 'cursor';
  document.getElementById('tab-scroll').hidden = btn.dataset.tab !== 'scroll';
});
const spage = document.getElementById('spage');
const slong = document.getElementById('slong');
slong.style.height = '14000px';
slong.style.position = 'relative';
slong.style.background = 'repeating-linear-gradient(#fff 0 99px, #eee 99px 100px)';
for (let y = 0; y < 14000; y += 500) {
  const m = document.createElement('div');
  Object.assign(m.style, { position: 'absolute', top: (y + 48) + 'px', left: '16px', font: '600 13px system-ui', color: '#5b5770' });
  m.textContent = y + ' px';
  slong.append(m);
}
const sinfo = document.getElementById('sinfo');
let sIdx = 0;
const startAt = (views) => (views < 0 ? 6000 : 0);
document.getElementById('splay').onclick = () => {
  const s = +document.getElementById('ssc').value;
  const v = +document.getElementById('svar').value;
  const plan = SCROLL[v][s][sIdx++ % SCROLL[v][s].length];
  spage.scrollTo({ top: startAt(plan.f[plan.f.length - 1]), behavior: 'instant' });
  const t0 = performance.now();
  sinfo.textContent = 'tocando ' + Math.round(plan.duration) + ' ms…';
  vitralScroll(spage, plan, (r) => { sinfo.textContent = r + ' em ' + Math.round(performance.now() - t0) + ' ms, posição ' + Math.round(spage.scrollTop); });
};
// Como era: passos secos de 100 px a cada ~85 ms (a roda via CDP não anima).
document.getElementById('sold').onclick = () => {
  const s = +document.getElementById('ssc').value;
  const plan = SCROLL[0][s][0];
  const total = plan.f[plan.f.length - 1];
  spage.scrollTo({ top: startAt(total), behavior: 'instant' });
  let done = 0;
  const tick = () => {
    const step = Math.sign(total) * Math.min(100, Math.abs(total) - done);
    spage.scrollTo({ top: spage.scrollTop + step, behavior: 'instant' });
    done += Math.abs(step);
    if (done < Math.abs(total)) setTimeout(tick, 85);
  };
  tick();
};
`
