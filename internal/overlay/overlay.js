// Overlay do Vitral: selo, pausa, destaque e cursor visual. Injetado em todo documento novo, só no
// frame principal.
// Tudo fica num único <vitral-overlay> com shadow root fechado: a página e o snapshot não enxergam.
// Textos e cores vêm do Go (estado renderizado e custom properties); nada é buscado na rede.
(() => {
  if (window.top !== window || window.__vitralOverlay) return;

  const TOKEN = __TOKEN__;
  const CSS_VARS = __CSS_VARS__;
  const MINIMAL = __MINIMAL__;
  const BINDING = '__vitral_event';

  // Tocador do cursor (cursor.js), no mesmo escopo: a página não alcança nada disso.
  __CURSOR__
  __SCROLL__

  let host = null;
  let root = null;
  let state = null;
  let hidden = false;

  const send = (type, id, result) => {
    try {
      const fn = window[BINDING];
      if (typeof fn === 'function') fn(JSON.stringify({ token: TOKEN, type, id, result }));
    } catch (_) { /* sem binding: nada a fazer */ }
  };

  const CSS = `
    :host { ${CSS_VARS} }
    .dock { position: fixed; right: 16px; bottom: 16px; pointer-events: none;
      font: 500 14px/1.3 system-ui, sans-serif; color: var(--vitral-paper); }
    .pill { display: flex; align-items: center; gap: 8px; min-height: 44px; box-sizing: border-box;
      padding: 0 4px 0 12px; border-radius: 999px; background: var(--vitral-ink);
      box-shadow: 0 4px 18px rgba(0, 0, 0, .28); pointer-events: auto; border: 2px solid transparent; }
    .dock.blocked .pill { border-color: var(--vitral-mark4); }
    .text { white-space: nowrap; max-width: 420px; overflow: hidden; text-overflow: ellipsis; }
    button { all: unset; box-sizing: border-box; display: inline-flex; align-items: center; justify-content: center;
      min-height: 44px; min-width: 44px; border-radius: 999px; cursor: pointer; pointer-events: auto;
      font: 600 14px/1 system-ui, sans-serif; }
    button:focus-visible { outline: 3px solid var(--vitral-primary); outline-offset: 2px; }
    .pause { padding: 0 16px; background: var(--vitral-paper); color: var(--vitral-ink); }
    .collapse { color: var(--vitral-paper); font-size: 20px; }
    .fab { width: 52px; height: 52px; background: var(--vitral-ink);
      box-shadow: 0 4px 18px rgba(0, 0, 0, .28); }
    .icon { width: 22px; height: 22px; flex: none; display: block; }
    .fab .icon { width: 28px; height: 28px; }
    .ring { position: fixed; box-sizing: border-box; border: 3px solid; border-radius: 6px;
      pointer-events: none; transition: opacity 400ms ease; }
    .ring.fade { opacity: 0; }
    .ring .tag { position: absolute; left: -3px; top: -14px; padding: 2px 7px; border-radius: 6px;
      font: 500 13px/1.2 ui-monospace, monospace; }
    .cur { position: fixed; inset: 0; pointer-events: none; overflow: hidden; }
    ${VITRAL_CURSOR_CSS}
  `;

  const ICON = VITRAL_ICON;

  const el = (tag, cls, text) => {
    const n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  };

  function hostStyle() {
    const vis = hidden ? 'hidden' : 'visible';
    return 'all: initial !important; display: block !important; position: fixed !important; inset: 0 !important;' +
      'z-index: 2147483647 !important; pointer-events: none !important; contain: strict !important;' +
      `visibility: ${vis} !important;`;
  }

  function mount() {
    if (host && host.isConnected) return;
    const de = document.documentElement;
    if (!de) return;
    if (!host) {
      host = document.createElement('vitral-overlay');
      root = host.attachShadow({ mode: 'closed' });
      const style = el('style');
      style.textContent = CSS;
      root.append(style, el('div', 'hl'), el('div', 'cur'), el('div', 'dock'));
      // Botões não roubam o foco da página nem deixam o clique vazar para os listeners dela.
      root.addEventListener('mousedown', (e) => e.preventDefault());
      root.addEventListener('click', (e) => {
        // Só clique de verdade (mouse/teclado da pessoa); eventos sintéticos não contam.
        if (!e.isTrusted) return;
        const target = e.target.closest && e.target.closest('[data-action]');
        if (!target) return;
        e.stopPropagation();
        send(target.dataset.action);
      });
    }
    host.setAttribute('style', hostStyle());
    de.appendChild(host);
    if (state) render(state);
    send('ready');
  }

  let lastKey = '';
  function render(s) {
    state = s;
    if (!root) return;
    const dock = root.querySelector('.dock');
    // Só redesenha se algo mudou: evita recriar botões sob o mouse da pessoa.
    const key = JSON.stringify(s);
    if (key === lastKey && dock.childElementCount > 0) return;
    lastKey = key;
    dock.className = 'dock' + (s.blocked ? ' blocked' : '');
    dock.replaceChildren();
    if (s.collapsed) {
      const fab = el('button', 'fab');
      fab.dataset.action = 'expand';
      fab.tabIndex = -1;
      fab.setAttribute('aria-label', s.expandLabel);
      fab.innerHTML = ICON;
      dock.append(fab);
    } else {
      const pill = el('div', 'pill');
      pill.setAttribute('role', 'status');
      pill.setAttribute('aria-live', 'polite');
      pill.insertAdjacentHTML('beforeend', ICON);
      if (!MINIMAL) pill.append(el('span', 'text', s.label));
      const pause = el('button', 'pause', s.button);
      pause.dataset.action = 'toggle_pause';
      pause.tabIndex = -1;
      const collapse = el('button', 'collapse', '–');
      collapse.dataset.action = 'collapse';
      collapse.tabIndex = -1;
      collapse.setAttribute('aria-label', s.collapseLabel);
      pill.append(pause, collapse);
      dock.append(pill);
    }
    const hl = root.querySelector('.hl');
    hl.querySelectorAll('.ring.hold').forEach((n) => n.remove());
    if (s.hold && !MINIMAL) hl.append(ring(s.hold, true));
    if (cur) cur.paused(s.blocked);
  }

  // Rolagem humana: toca o plano do Go no container ('page' ou a chave dada pelo scrollChain) e avisa
  // scroll_done. Se a pessoa rolar por conta própria (roda, trackpad, toque ou teclado de verdade),
  // para na hora com a posição final do plano: quem interrompe a IA é a pausa.
  function scroll(c) {
    const el = c.container === 'page' ? document.scrollingElement
      : (window.__vitral && window.__vitral.handle ? window.__vitral.handle(c.container) : null);
    if (!el) return send('scroll_done', c.id, 'missing');
    let ctl = null;
    const keys = new Set(['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown', 'Home', 'End', ' ']);
    const onUser = (e) => {
      if (!e.isTrusted || (e.type === 'keydown' && !keys.has(e.key))) return;
      if (ctl) ctl.stop();
    };
    const types = ['wheel', 'touchstart', 'keydown'];
    types.forEach((t) => window.addEventListener(t, onUser, { capture: true, passive: true }));
    ctl = vitralScroll(el, c.plan, (result) => {
      types.forEach((t) => window.removeEventListener(t, onUser, { capture: true }));
      send('scroll_done', c.id, result);
    });
  }

  // Cursor visual (docs/CURSOR.md). Criado na primeira ordem do Go; nunca no modo mínimo.
  let cur = null;
  const reducedMotion = () => window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  function cursor(c) {
    if (!root || MINIMAL) {
      if (c.op === 'play') send('cursor_done', c.id);
      return;
    }
    const layer = root.querySelector('.cur');
    if (!cur) {
      cur = vitralCursor(layer, c.label || 'IA');
      if (state) cur.paused(state.blocked);
    }
    switch (c.op) {
      case 'play':
        cur.play(c.plan, {
          ring: c.ring, reduced: reducedMotion(), appear: c.appear,
          arrive: () => send('cursor_done', c.id),
        });
        break;
      case 'place': cur.place(Math.min(Math.max(c.x, 0), innerWidth - 1), Math.min(Math.max(c.y, 0), innerHeight - 1), c.visible !== false); break;
      case 'nav': cur.nav(c.on); break;
      case 'hide': cur.hide(); break;
      case 'note': cur.note(c.text, c.ms); break;
      case 'fail': cur.fail(c.text, reducedMotion()); break;
      case 'keycap': vitralKeycap(layer, c.text); break;
    }
  }

  // ring desenha o contorno 6 px fora da caixa, como no screenshot (docs/BRAND.md, seção 5.1).
  function ring(h, hold) {
    const r = el('div', 'ring' + (hold ? ' hold' : ''));
    const [x, y, w, hgt] = h.box;
    Object.assign(r.style, {
      left: `${x - 6}px`, top: `${y - 6}px`, width: `${w + 12}px`, height: `${hgt + 12}px`,
      borderColor: `var(--vitral-mark${h.mark})`,
    });
    const tag = el('span', 'tag', h.ref);
    tag.style.background = `var(--vitral-mark${h.mark})`;
    tag.style.color = `var(--vitral-${h.text})`;
    r.append(tag);
    return r;
  }

  function highlight(h) {
    if (!root || MINIMAL) return;
    const hl = root.querySelector('.hl');
    hl.querySelectorAll('.ring:not(.hold)').forEach((n) => n.remove());
    const r = ring(h, false);
    hl.append(r);
    setTimeout(() => {
      r.classList.add('fade');
      setTimeout(() => r.remove(), 400);
    }, h.ms);
  }

  function setHidden(v) {
    hidden = !!v;
    if (host) host.setAttribute('style', hostStyle());
  }

  Object.defineProperty(window, '__vitralOverlay', {
    value: Object.freeze({ render, highlight, hidden: setHidden, cursor, scroll }),
    enumerable: false,
  });

  // Recoloca o host se a página o remover (ex.: document.open ou troca do <html>) e desfaz
  // tentativas de escondê-lo pelo atributo style (o JS da página enxerga o elemento no DOM).
  new MutationObserver(() => {
    if (!host || !host.isConnected) {
      mount();
      return;
    }
    if (host.getAttribute('style') !== hostStyle()) host.setAttribute('style', hostStyle());
  }).observe(document, { childList: true, subtree: true, attributes: true, attributeFilter: ['style', 'hidden', 'class'] });

  if (document.documentElement) mount();
  else document.addEventListener('readystatechange', mount, { once: true });
})();
