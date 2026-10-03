// Overlay do Vitral: selo, pausa e destaque. Injetado em todo documento novo, só no frame principal.
// Tudo fica num único <vitral-overlay> com shadow root fechado: a página e o snapshot não enxergam.
// Textos e cores vêm do Go (estado renderizado e custom properties); nada é buscado na rede.
(() => {
  if (window.top !== window || window.__vitralOverlay) return;

  const TOKEN = __TOKEN__;
  const CSS_VARS = __CSS_VARS__;
  const MINIMAL = __MINIMAL__;
  const BINDING = '__vitral_event';

  let host = null;
  let root = null;
  let state = null;
  let hidden = false;

  const send = (type) => {
    try {
      const fn = window[BINDING];
      if (typeof fn === 'function') fn(JSON.stringify({ token: TOKEN, type }));
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
  `;

  // Ícone pequeno (icon-small.svg) para fundo escuro: a moldura vira vidro claro.
  const ICON = `<svg class="icon" viewBox="0 0 512 512" aria-hidden="true">
    <defs><clipPath id="vitral-glass"><path d="M124 436 V208 A132 132 0 0 1 388 208 V436 Z"/></clipPath></defs>
    <path d="M96 464 V208 A160 160 0 0 1 416 208 V464 Z" style="fill: var(--vitral-paper)"/>
    <g clip-path="url(#vitral-glass)">
      <rect x="124" y="40" width="120" height="180" style="fill: var(--vitral-mark1)"/>
      <rect x="268" y="40" width="120" height="180" style="fill: var(--vitral-mark4)"/>
      <rect x="124" y="244" width="120" height="192" style="fill: var(--vitral-mark3)"/>
      <rect x="268" y="244" width="120" height="192" style="fill: var(--vitral-mark2)"/>
    </g></svg>`;

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
      root.append(style, el('div', 'hl'), el('div', 'dock'));
      // Botões não roubam o foco da página nem deixam o clique vazar para os listeners dela.
      root.addEventListener('mousedown', (e) => e.preventDefault());
      root.addEventListener('click', (e) => {
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
    value: Object.freeze({ render, highlight, hidden: setHidden }),
    enumerable: false,
  });

  // Recoloca o host se a página o remover (ex.: document.open ou troca do <html>).
  new MutationObserver(() => {
    if (!host || !host.isConnected) mount();
  }).observe(document, { childList: true, subtree: true });

  if (document.documentElement) mount();
  else document.addEventListener('readystatechange', mount, { once: true });
})();
