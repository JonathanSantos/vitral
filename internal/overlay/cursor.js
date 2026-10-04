// Cursor visual da IA (docs/CURSOR.md). Só toca o plano calculado pelo Go (internal/cursor):
// interpola os keyframes, troca a forma, faz press, anel e etiqueta. Nenhuma física aqui.
// Usado pelo overlay (isolated world, shadow root fechado) e pelo tools/cursorlab.
// Cores vêm das custom properties --vitral-* (internal/brand); nada é buscado na rede.

const VITRAL_CURSOR_CSS = `
  .vc { position: absolute; left: 0; top: 0; pointer-events: none; will-change: transform;
    transition: opacity 150ms ease; z-index: 2; }
  .vc.gone { opacity: 0; }
  .vc.dim { opacity: .7; transition: opacity 600ms ease; }
  .vc.nav { opacity: .4; }
  .vc.paused { opacity: 0; transition: opacity 200ms ease; }
  .vc-body { position: absolute; left: 0; top: 0; width: 20px; height: 24px;
    transform-origin: var(--hx) var(--hy); transition: transform 60ms ease;
    filter: drop-shadow(0 2px 4px rgba(0, 0, 0, .25)); }
  .vc-body.down { transform: scale(.9); }
  .vc-body svg { display: block; width: 20px; height: 24px; overflow: visible; }
  .vc-body path { fill: var(--vitral-ink); stroke: var(--vitral-paper); stroke-width: 1.5;
    stroke-linejoin: round; paint-order: stroke; }
  .vc-tag { position: absolute; left: 16px; top: 22px; display: flex; align-items: center; gap: 3px;
    padding: 2px 6px 2px 3px; border-radius: 999px; background: var(--vitral-ink); color: var(--vitral-paper);
    font: 600 10px/1 system-ui, sans-serif; white-space: nowrap; transition: opacity 400ms ease; }
  .vc-tag.off { opacity: 0; }
  .vc-tag svg { width: 12px; height: 12px; display: block; }
  .vc-note { position: absolute; left: 16px; top: 42px; padding: 4px 9px; border-radius: 8px;
    background: var(--vitral-ink); color: var(--vitral-paper); font: 500 12px/1.3 system-ui, sans-serif;
    white-space: nowrap; box-shadow: 0 2px 8px rgba(0, 0, 0, .2); transition: opacity 200ms ease; }
  .vc-keycap { position: absolute; left: 50%; bottom: 84px; transform: translateX(-50%); min-width: 28px;
    padding: 8px 14px; border-radius: 10px; background: var(--vitral-ink); color: var(--vitral-paper);
    border: 2px solid var(--vitral-paper); box-shadow: 0 4px 18px rgba(0, 0, 0, .28); text-align: center;
    font: 600 15px/1 system-ui, sans-serif; pointer-events: none; transition: opacity 200ms ease; }
  .vc-ring { position: absolute; left: 0; top: 0; width: 56px; height: 56px; margin: -28px 0 0 -28px;
    border: 2px solid; border-radius: 50%; box-sizing: border-box; pointer-events: none; z-index: 1; }
`;

// Formas em 20×24 com o ponto quente (hx, hy) onde o clique acontece.
const VITRAL_CURSOR_SHAPES = {
  arrow: { hx: 2, hy: 1.5, d: 'M2 1.5V19.5L6.6 15.2 9.6 22 12.6 20.7 9.7 14.1H16Z' },
  pointer: { hx: 8.6, hy: 0.8, d: 'M7 2.4c0-.9.7-1.6 1.6-1.6s1.6.7 1.6 1.6V9h.3c.9 0 1.6.6 1.6 1.4v.2c.2-.6.8-1 1.5-1 .9 0 1.6.7 1.6 1.5v.6c.3-.4.8-.6 1.3-.6.9 0 1.5.7 1.5 1.5v4.8c0 3.6-2.7 6.4-6.2 6.4h-1.4c-2 0-3.6-1-4.6-2.6L2.4 16c-.5-.8-.3-1.8.5-2.2.7-.4 1.6-.2 2.1.5L7 16.5Z' },
  text: { hx: 10, hy: 12, d: 'M6 2h3l1 1 1-1h3v1.6h-2.4c-.6 0-.9.3-.9.9v6.8h1.6v1.4h-1.6v6.8c0 .6.3.9.9.9H14V22h-3l-1-1-1 1H6v-1.6h2.4c.6 0 .9-.3.9-.9v-6.8H7.7v-1.4h1.6V4.5c0-.6-.3-.9-.9-.9H6Z' },
};

// Ícone pequeno do Vitral (icon-small.svg) para fundo escuro: a moldura vira vidro claro. Usado no
// selo e na etiqueta "IA" do cursor.
const VITRAL_ICON = `<svg class="icon" viewBox="0 0 512 512" aria-hidden="true">
  <defs><clipPath id="vitral-glass"><path d="M124 436 V208 A132 132 0 0 1 388 208 V436 Z"/></clipPath></defs>
  <path d="M96 464 V208 A160 160 0 0 1 416 208 V464 Z" style="fill: var(--vitral-paper); stroke: none"/>
  <g clip-path="url(#vitral-glass)" style="stroke: none">
    <rect x="124" y="40" width="120" height="180" style="fill: var(--vitral-mark1)"/>
    <rect x="268" y="40" width="120" height="180" style="fill: var(--vitral-mark4)"/>
    <rect x="124" y="244" width="120" height="192" style="fill: var(--vitral-mark3)"/>
    <rect x="268" y="244" width="120" height="192" style="fill: var(--vitral-mark2)"/>
  </g></svg>`;

// vitralCursor desenha o cursor dentro de layer (um elemento posicionado que cobre a área das
// coordenadas: no overlay, fixed na viewport inteira). label é o texto da etiqueta ("IA").
function vitralCursor(layer, label) {
  const el = document.createElement('div');
  el.className = 'vc gone';
  const body = document.createElement('div');
  body.className = 'vc-body';
  const tag = document.createElement('div');
  tag.className = 'vc-tag off';
  tag.innerHTML = VITRAL_ICON;
  tag.append(document.createTextNode(label));
  el.append(body, tag);
  layer.append(el);

  let x = 0, y = 0, shape = '', raf = 0, current = null, idleTimers = [];

  const setShape = (name) => {
    if (name === shape) return;
    shape = name;
    const s = VITRAL_CURSOR_SHAPES[name] || VITRAL_CURSOR_SHAPES.arrow;
    body.innerHTML = `<svg viewBox="0 0 20 24"><path d="${s.d}"/></svg>`;
    body.style.setProperty('--hx', `${s.hx}px`);
    body.style.setProperty('--hy', `${s.hy}px`);
    body.dataset.hx = s.hx;
    body.dataset.hy = s.hy;
    move(x, y);
  };
  const move = (nx, ny) => {
    x = nx; y = ny;
    el.style.transform = `translate(${x - Number(body.dataset.hx || 0)}px, ${y - Number(body.dataset.hy || 0)}px)`;
  };
  const awake = () => {
    idleTimers.forEach(clearTimeout);
    idleTimers = [];
    el.classList.remove('dim');
    tag.classList.remove('off');
  };
  // Parado: a etiqueta some em 1,5 s e o cursor vai a 70% em 5 s (6.1).
  const idle = () => {
    idleTimers.push(setTimeout(() => tag.classList.add('off'), 1500));
    idleTimers.push(setTimeout(() => el.classList.add('dim'), 5000));
  };
  const ring = (color, reduced) => {
    const r = document.createElement('div');
    r.className = 'vc-ring';
    r.style.borderColor = color;
    r.style.transform = `translate(${x}px, ${y}px)`;
    layer.append(r);
    const anim = reduced
      ? r.animate([{ opacity: 0.6, transform: `translate(${x}px, ${y}px) scale(.5)` }, { opacity: 0.6, transform: `translate(${x}px, ${y}px) scale(.5)` }], { duration: 200 })
      : r.animate([{ opacity: 0.6, transform: `translate(${x}px, ${y}px) scale(0)` }, { opacity: 0, transform: `translate(${x}px, ${y}px) scale(1)` }], { duration: 300, easing: 'ease-out' });
    anim.onfinish = () => r.remove();
  };
  setShape('arrow');

  // frameAt interpola linearmente os keyframes [t, x, y, ...] no instante t.
  const frameAt = (f, t, from) => {
    let i = from;
    while (i + 3 < f.length && f[i + 3] <= t) i += 3;
    if (i + 3 >= f.length) return [f[f.length - 2], f[f.length - 1], i];
    const k = (t - f[i]) / (f[i + 3] - f[i]);
    return [f[i + 1] + (f[i + 4] - f[i + 1]) * k, f[i + 2] + (f[i + 5] - f[i + 2]) * k, i];
  };

  // finish termina o plano na hora: posição final, sem press pendente, e avisa.
  const finish = (plan, done) => {
    cancelAnimationFrame(raf);
    const f = plan.f;
    setShape(plan.shape);
    move(f[f.length - 2], f[f.length - 1]);
    body.classList.remove('down');
    el.classList.remove('gone');
    current = null;
    idle();
    done && done();
  };

  return {
    // place põe o cursor num ponto sem animar (reidratação após navegação, primeira aparição).
    place(px, py, visible = true) {
      move(px, py);
      el.classList.toggle('gone', !visible);
    },
    get position() { return { x, y }; },
    // play toca o plano. opts.ring é a cor do anel (null = sem clique); opts.reduced deixa o anel
    // estático; opts.appear faz o cursor surgir com fade (primeira aparição, a partir do selo);
    // opts.arrive é chamado no press (ou no fim do movimento, sem clique): é quando o Go dispara o
    // evento real. done vem depois do release. Com a aba oculta, os dois vêm na hora.
    play(plan, opts, done) {
      if (current) finish(current.plan, current.done);
      let arrived = false;
      const arrive = () => {
        if (!arrived) { arrived = true; opts.arrive && opts.arrive(); }
      };
      const userDone = done;
      done = () => { arrive(); userDone && userDone(); };
      awake();
      el.classList.remove('gone', 'nav');
      if (opts.appear) el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 150 });
      const f = plan.f;
      if (document.visibilityState !== 'visible') return finish(plan, done);
      current = { plan, done };
      let start = 0, i = 0, pressed = false;
      if (plan.fade > 0 || f.length === 3) {
        move(f[1], f[2]);
        setShape(plan.shape);
        if (plan.fade > 0) el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: plan.fade });
      }
      const onHide = () => {
        if (document.visibilityState !== 'visible' && current && current.plan === plan) finish(plan, done);
      };
      document.addEventListener('visibilitychange', onHide);
      const step = (now) => {
        if (!current || current.plan !== plan) return document.removeEventListener('visibilitychange', onHide);
        if (!start) start = now;
        const t = now - start;
        if (plan.fade === 0 && f.length > 3) {
          const [nx, ny, ni] = frameAt(f, Math.min(t, plan.move), i);
          i = ni;
          move(nx, ny);
          // Saindo do alvo anterior: volta para a seta logo no começo.
          setShape(t >= plan.shapeAt ? plan.shape : (t > 60 ? 'arrow' : shape));
        }
        if (opts.ring && !pressed && t >= plan.press) {
          pressed = true;
          body.classList.add('down');
          ring(opts.ring, opts.reduced);
          arrive();
        }
        if (!opts.ring && t >= plan.move) arrive();
        if (t >= plan.release || (!opts.ring && t >= plan.move)) {
          document.removeEventListener('visibilitychange', onHide);
          return finish(plan, done);
        }
        raf = requestAnimationFrame(step);
      };
      raf = requestAnimationFrame(step);
    },
    hide() { el.classList.add('gone'); },
    show() { el.classList.remove('gone'); },
    // Pausa: a pessoa está no controle, o cursor some em 200 ms e volta ao retomar (6.2).
    paused(on) { el.classList.toggle('paused', !!on); },
    // Navegação (open/back): o cursor fica a 40% até a página carregar (5.6).
    nav(on) { el.classList.toggle('nav', !!on); },
    // note mostra uma etiqueta curta junto ao cursor ("Selecionado: …", "Botão desabilitado").
    note(text, ms) {
      el.querySelectorAll('.vc-note').forEach((n) => n.remove());
      const n = document.createElement('div');
      n.className = 'vc-note';
      n.textContent = text;
      el.append(n);
      setTimeout(() => { n.style.opacity = '0'; setTimeout(() => n.remove(), 200); }, ms);
    },
    // fail é o "não" com a cabeça: 3 oscilações de 6 px em 240 ms e a etiqueta por 1,2 s (6.4).
    fail(text, reduced) {
      awake();
      el.classList.remove('gone');
      if (!reduced) {
        body.parentElement.animate(
          [0, -6, 6, -6, 6, -6, 0].map((dx) => ({ transform: `${el.style.transform} translateX(${dx}px)` })),
          { duration: 240, easing: 'ease-in-out' });
      }
      this.note(text, 1200);
      idle();
    },
  };
}

// vitralKeycap mostra a tecla pressionada no centro inferior, acima do selo, por 600 ms (5.5).
function vitralKeycap(layer, text) {
  layer.querySelectorAll('.vc-keycap').forEach((n) => n.remove());
  const k = document.createElement('div');
  k.className = 'vc-keycap';
  k.textContent = text;
  layer.append(k);
  setTimeout(() => { k.style.opacity = '0'; setTimeout(() => k.remove(), 200); }, 600);
}
