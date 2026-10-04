// Injetado em todo documento novo. Expõe window.__vitral (e window.__vitralSnapshot).
// Só coleta dados: formatação, compactação e diff acontecem em Go.
(() => {
  if (window.__vitral) return;

  const REF_ATTR = 'data-vitral-ref';
  const NAME_MAX = 300;
  const state = {
    counter: 0,
    refs: new Map(),
    docId: Math.random().toString(36).slice(2, 10),
    lastMutation: performance.now(),
  };

  // Observa mutações para saber quando o DOM "assentou". Ignora as do próprio data-vitral-ref.
  // Mudanças do overlay do Vitral (selo, destaque) não contam como a página "se mexendo".
  const OVERLAY_TAG = 'VITRAL-OVERLAY';
  const isOverlay = (n) => n && n.nodeName === OVERLAY_TAG;
  const isOverlayRecord = (r) => {
    if (isOverlay(r.target)) return true;
    if (r.type !== 'childList') return false;
    const nodes = [...r.addedNodes, ...r.removedNodes];
    return nodes.length > 0 && nodes.every(isOverlay);
  };

  const observe = () => {
    try {
      new MutationObserver((records) => {
        for (const r of records) {
          if (r.type === 'attributes' && r.attributeName === REF_ATTR) continue;
          if (isOverlayRecord(r)) continue;
          state.lastMutation = performance.now();
          return;
        }
      }).observe(document, { subtree: true, childList: true, attributes: true, characterData: true });
    } catch (_) { /* documento sem suporte: estabilidade cai só na rede */ }
  };
  observe();

  const INTERACTIVE_ROLES = new Set([
    'button', 'link', 'checkbox', 'radio', 'switch', 'tab', 'menuitem', 'menuitemcheckbox',
    'menuitemradio', 'option', 'combobox', 'textbox', 'searchbox', 'slider', 'spinbutton', 'treeitem',
  ]);
  const STRUCTURE_ROLES = new Set([
    'dialog', 'alertdialog', 'alert', 'status', 'heading', 'table', 'grid', 'row', 'list', 'listbox',
    'listitem', 'menu', 'menubar', 'tablist', 'tabpanel', 'tree', 'navigation', 'main', 'banner',
    'contentinfo', 'complementary', 'form', 'region', 'search', 'group', 'label', 'error',
  ]);
  const COUNTED = { table: 'row', grid: 'row', list: 'listitem', listbox: 'option', menu: 'menuitem', tree: 'treeitem', combobox: 'option' };
  const SKIP_TAGS = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'HEAD', 'META', 'LINK', 'svg', 'SVG', 'VITRAL-OVERLAY']);
  const TEXT_INPUTS = new Set(['', 'text', 'email', 'tel', 'url', 'password', 'date', 'datetime-local', 'month', 'time', 'week']);
  const ERROR_CLASS = /(^|[\s_-])(error|errors|invalid|danger)([\s_-]|$)/i;

  const norm = (s) => (s || '').replace(/\s+/g, ' ').trim();
  // Corta em NAME_MAX caracteres (não unidades UTF-16) e marca o corte com "…": sem a marca, um texto
  // cortado pareceria completo no snapshot com scope.
  const cut = (s) => {
    if (s.length <= NAME_MAX) return s;
    const chars = Array.from(s);
    return chars.length > NAME_MAX ? chars.slice(0, NAME_MAX - 1).join('') + '…' : s;
  };

  function refFor(el) {
    const existing = el.getAttribute(REF_ATTR);
    if (existing && state.refs.get(existing)?.deref() === el) return existing;
    const ref = 'e' + (++state.counter);
    el.setAttribute(REF_ATTR, ref);
    state.refs.set(ref, new WeakRef(el));
    return ref;
  }

  function find(ref) {
    const el = state.refs.get(ref)?.deref();
    if (el && el.isConnected) return el;
    return null;
  }

  function implicitRole(el) {
    const tag = el.tagName;
    const type = (el.getAttribute('type') || '').toLowerCase();
    switch (tag) {
      case 'A': case 'AREA': return el.hasAttribute('href') ? 'link' : '';
      case 'BUTTON': return 'button';
      case 'SUMMARY': return 'button';
      case 'TEXTAREA': return 'textbox';
      case 'SELECT': return el.multiple || el.size > 1 ? 'listbox' : 'combobox';
      case 'OPTION': return 'option';
      case 'H1': case 'H2': case 'H3': case 'H4': case 'H5': case 'H6': return 'heading';
      case 'DIALOG': return 'dialog';
      case 'TABLE': return 'table';
      case 'TR': return 'row';
      case 'UL': case 'OL': return 'list';
      case 'LI': return 'listitem';
      case 'NAV': return 'navigation';
      case 'MAIN': return 'main';
      case 'ASIDE': return 'complementary';
      case 'FORM': return 'form';
      case 'SEARCH': return 'search';
      case 'FIELDSET': return 'group';
      case 'LABEL': return 'label';
      case 'SECTION': return el.hasAttribute('aria-label') || el.hasAttribute('aria-labelledby') ? 'region' : '';
      case 'HEADER': return el.closest('article,aside,main,nav,section') ? '' : 'banner';
      case 'FOOTER': return el.closest('article,aside,main,nav,section') ? '' : 'contentinfo';
      case 'INPUT':
        if (type === 'hidden') return '';
        if (['button', 'submit', 'reset', 'image', 'file', 'color'].includes(type)) return 'button';
        if (type === 'checkbox') return 'checkbox';
        if (type === 'radio') return 'radio';
        if (type === 'range') return 'slider';
        if (type === 'number') return 'spinbutton';
        if (type === 'search') return 'searchbox';
        return 'textbox';
    }
    if (el.isContentEditable && !el.parentElement?.isContentEditable) return 'textbox';
    return '';
  }

  function roleOf(el) {
    const explicit = (el.getAttribute('role') || '').trim().split(/\s+/)[0];
    if (explicit === 'presentation' || explicit === 'none') return '';
    if (explicit) return explicit === 'alertdialog' ? 'dialog' : explicit;
    const implicit = implicitRole(el);
    if (implicit) return implicit;
    if (isErrorText(el)) return 'error';
    const live = el.getAttribute('aria-live');
    if ((live === 'polite' || live === 'assertive') && norm(el.innerText)) return 'status';
    if (el.hasAttribute('onclick') || el.tabIndex >= 0 && el.hasAttribute('tabindex')) return 'clickable';
    return '';
  }

  function isErrorText(el) {
    const cls = typeof el.className === 'string' ? el.className : '';
    if (!cls || !ERROR_CLASS.test(cls)) return false;
    if (el.querySelector('input,select,textarea,button,a[href]')) return false;
    const text = norm(el.innerText);
    return text.length > 0 && text.length < 300;
  }

  // Elementos com cursor:pointer cujo pai não tem pointer costumam ser clicáveis via addEventListener.
  function isPointerClickable(el, style) {
    if (style.cursor !== 'pointer') return false;
    const parent = el.parentElement;
    if (parent && getComputedStyle(parent).cursor === 'pointer') return false;
    return norm(el.innerText).length > 0 || el.querySelector('img,svg') !== null;
  }

  function textOf(el) {
    if (!el.querySelector('select,textarea')) return norm(el.innerText ?? el.textContent);
    // innerText incluiria as opções do select; percorre os nós ignorando controles.
    const parts = [];
    const walk = (n) => {
      for (const c of n.childNodes) {
        if (c.nodeType === Node.TEXT_NODE) parts.push(c.textContent);
        else if (c.nodeType === Node.ELEMENT_NODE && !['SELECT', 'TEXTAREA', 'SCRIPT', 'STYLE'].includes(c.tagName)) walk(c);
      }
    };
    walk(el);
    return norm(parts.join(' '));
  }

  function byIds(el, attr) {
    const ids = (el.getAttribute(attr) || '').split(/\s+/).filter(Boolean);
    const root = el.getRootNode();
    return ids.map((id) => root.getElementById?.(id) || el.ownerDocument.getElementById(id)).filter(Boolean);
  }

  function nameOf(el, role) {
    const labelled = byIds(el, 'aria-labelledby');
    if (labelled.length) return cut(norm(labelled.map((n) => textOf(n)).join(' ')));
    const aria = norm(el.getAttribute('aria-label'));
    if (aria) return cut(aria);

    const tag = el.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') {
      const type = (el.getAttribute('type') || '').toLowerCase();
      if (['button', 'submit', 'reset'].includes(type)) return cut(norm(el.value) || type);
      if (type === 'image') return cut(norm(el.alt));
      const fromLabels = el.labels ? norm([...el.labels].map((l) => textOf(l)).join(' ')) : '';
      return cut(fromLabels || norm(el.placeholder) || norm(el.title));
    }
    switch (role) {
      case 'row': return cut(rowText(el));
      case 'table': case 'grid': return cut(norm(el.caption?.innerText) || norm(el.title));
      case 'dialog': {
        const h = el.querySelector('h1,h2,h3,h4,h5,h6,[role=heading]');
        return cut(h ? textOf(h) : '');
      }
      case 'group': {
        const legend = tag === 'FIELDSET' ? el.querySelector(':scope > legend') : null;
        return cut(legend ? textOf(legend) : '');
      }
      case 'list': case 'navigation': case 'main': case 'banner': case 'contentinfo':
      case 'complementary': case 'form': case 'region': case 'search': case 'menu': case 'menubar':
      case 'tablist': case 'tree': case 'listbox': case 'tabpanel':
        return '';
    }
    const text = textOf(el);
    if (text) return cut(text);
    const img = el.querySelector('img[alt]');
    return cut(norm(img?.alt) || norm(el.title));
  }

  function rowText(row) {
    const cells = [...row.children].filter((c) => ['TD', 'TH'].includes(c.tagName) || ['cell', 'gridcell', 'columnheader', 'rowheader'].includes(c.getAttribute('role')));
    if (!cells.length) return textOf(row);
    return cells.map((c) => norm(c.innerText)).join(' | ');
  }

  function valueOf(el, role) {
    if (role === 'combobox' && el.tagName === 'SELECT') return [...el.selectedOptions].map((o) => norm(o.label)).join(', ');
    if (!['textbox', 'searchbox', 'spinbutton', 'slider', 'combobox'].includes(role)) return undefined;
    if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') {
      if (el.type === 'password') return el.value ? '••••' : '';
      return el.value;
    }
    if (el.isContentEditable) return norm(el.innerText);
    const v = el.getAttribute('aria-valuetext') ?? el.getAttribute('aria-valuenow');
    return v ?? undefined;
  }

  function statesOf(el, role) {
    const s = [];
    const aria = (n) => el.getAttribute('aria-' + n);
    const doc = el.ownerDocument;
    if (role === 'dialog') {
      s.push('open');
      try { if (el.matches(':modal') || aria('modal') === 'true') s.push('modal'); } catch (_) {}
    }
    try { if (el.matches(':disabled') || aria('disabled') === 'true') s.push('disabled'); } catch (_) {}
    if (['checkbox', 'radio', 'switch', 'menuitemcheckbox', 'menuitemradio'].includes(role)) {
      const ac = aria('checked');
      if (el.indeterminate || ac === 'mixed') s.push('checked=mixed');
      else if (el.checked || ac === 'true') s.push('checked');
    }
    if (el.tagName === 'SUMMARY') s.push(el.parentElement?.open ? 'expanded' : 'collapsed');
    else if (aria('expanded') === 'true') s.push('expanded');
    else if (aria('expanded') === 'false') s.push('collapsed');
    if (aria('selected') === 'true' || (role === 'option' && el.selected)) s.push('selected');
    if (aria('pressed') === 'true') s.push('pressed');
    if (aria('current') && aria('current') !== 'false') s.push('current');
    if (el.required || aria('required') === 'true') s.push('required');
    if (el.readOnly && ['textbox', 'searchbox', 'spinbutton'].includes(role)) s.push('readonly');
    let invalid = aria('invalid') === 'true';
    try { invalid = invalid || el.matches(':user-invalid'); } catch (_) {}
    if (invalid) s.push('invalid');
    if (doc.activeElement === el && el !== doc.body) s.push('focused');
    return s;
  }

  function countOf(el, role) {
    const child = COUNTED[role];
    if (!child) return undefined;
    if (el.tagName === 'SELECT') return el.options.length;
    if (el.tagName === 'TABLE') return [...el.tBodies].reduce((n, b) => n + b.rows.length, 0) || el.rows.length;
    if (el.tagName === 'UL' || el.tagName === 'OL') return [...el.children].filter((c) => c.tagName === 'LI').length;
    const ariaCount = Number(el.getAttribute(role === 'table' || role === 'grid' ? 'aria-rowcount' : 'aria-setsize'));
    if (ariaCount > 0) return ariaCount;
    return el.querySelectorAll(`[role=${child}]`).length;
  }

  function isVisible(el) {
    if (el.checkVisibility) return el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true });
    const st = getComputedStyle(el);
    return st.display !== 'none' && st.visibility !== 'hidden' && st.opacity !== '0';
  }

  // Elementos com display:none (ou ancestral) não têm nada visível na subárvore.
  function isRendered(el) {
    if (el.checkVisibility) return el.checkVisibility();
    return getComputedStyle(el).display !== 'none';
  }

  function frameOffset(doc) {
    let x = 0, y = 0;
    let win = doc.defaultView;
    while (win && win.frameElement) {
      const fe = win.frameElement;
      const r = fe.getBoundingClientRect();
      x += r.left + fe.clientLeft;
      y += r.top + fe.clientTop;
      win = fe.ownerDocument.defaultView;
    }
    return { x, y };
  }

  function boxOf(el) {
    const r = el.getBoundingClientRect();
    const off = frameOffset(el.ownerDocument);
    return [Math.round(r.left + off.x), Math.round(r.top + off.y), Math.round(r.width), Math.round(r.height)];
  }

  function nearViewport(box) {
    const vw = window.innerWidth, vh = window.innerHeight;
    const mx = vw * 0.25, my = vh * 0.25;
    return box[0] + box[2] >= -mx && box[0] <= vw + mx && box[1] + box[3] >= -my && box[1] <= vh + my;
  }

  // Filhos na árvore "achatada": shadow roots no lugar do host, slots resolvidos, iframes same-origin.
  function childrenOf(el) {
    if (el.shadowRoot) return [...el.shadowRoot.children];
    if (el.tagName === 'SLOT') {
      const assigned = el.assignedElements({ flatten: true });
      return assigned.length ? assigned : [...el.children];
    }
    if (el.tagName === 'IFRAME' || el.tagName === 'FRAME') {
      try {
        const body = el.contentDocument?.body;
        return body ? [body] : [];
      } catch (_) { return []; }
    }
    return [...el.children];
  }

  function shouldInclude(el, role, style) {
    if (INTERACTIVE_ROLES.has(role) || role === 'clickable') return true;
    if (role === 'label') return !el.control || !isVisible(el.control);
    if (role === 'group') return el.tagName === 'FIELDSET' || el.hasAttribute('aria-label') || el.hasAttribute('aria-labelledby');
    if (STRUCTURE_ROLES.has(role)) return true;
    return isPointerClickable(el, style);
  }

  function makeNode(el, role) {
    const node = { ref: refFor(el), role, name: nameOf(el, role) };
    const value = valueOf(el, role);
    if (value !== undefined) node.value = cut(value);
    const states = statesOf(el, role);
    if (states.length) node.states = states;
    if (states.includes('invalid') && el.validationMessage) node.invalidMessage = cut(norm(el.validationMessage));
    if (role === 'heading') {
      node.level = Number(el.getAttribute('aria-level')) || Number(el.tagName.slice(1)) || 2;
    }
    if (el.tagName === 'INPUT' && !['', 'text', 'checkbox', 'radio', 'button', 'submit', 'reset'].includes(el.type)) node.type = el.type;
    const count = countOf(el, role);
    if (count !== undefined) node.count = count;
    node.box = boxOf(el);
    return node;
  }

  function walk(el, out, opts) {
    if (SKIP_TAGS.has(el.tagName) || !isRendered(el)) return;
    const style = getComputedStyle(el);
    let role = roleOf(el);
    if (!role && style.cursor === 'pointer' && isPointerClickable(el, style)) role = 'clickable';
    const visible = isVisible(el);
    let target = out;

    if (role && visible && shouldInclude(el, role, style)) {
      const box = boxOf(el);
      const sized = box[2] > 0 && box[3] > 0;
      if (sized && (opts.full || nearViewport(box))) {
        const node = makeNode(el, role);
        out.push(node);
        if (el.tagName === 'SELECT') {
          node.children = [...el.options].map((o) => optionNode(o));
          return;
        }
        target = node.children = [];
      }
    }
    for (const child of childrenOf(el)) walk(child, target, opts);
    if (target !== out && target.length === 0) {
      const node = out[out.length - 1];
      delete node.children;
    }
  }

  // Options de <select> fechado não têm caixa, então entram sem checagem de visibilidade.
  function optionNode(o) {
    const node = { ref: refFor(o), role: 'option', name: cut(norm(o.label)), box: [0, 0, 0, 0] };
    const states = [];
    if (o.selected) states.push('selected');
    if (o.disabled) states.push('disabled');
    if (states.length) node.states = states;
    return node;
  }

  function isOffscreen(el) {
    // <option> de select fechado não tem caixa: vale o estado do select.
    const target = el.tagName === 'OPTION' ? el.closest('select') || el : el;
    return isVisible(target) && !nearViewport(boxOf(target));
  }

  function snapshot(opts) {
    opts = opts || {};
    let roots = [document.body || document.documentElement];
    let full = !!opts.full;
    if (opts.scope) {
      const scoped = find(opts.scope);
      if (!scoped) return { error: 'ref_not_found', ref: opts.scope };
      roots = [scoped];
      full = true;
    }
    const nodes = [];
    for (const r of roots) if (r) walk(r, nodes, { full });

    // Refs que sumiram do snapshot mas continuam visíveis fora da viewport (para o resumo do diff).
    const offscreen = [];
    for (const ref of opts.known || []) {
      const el = find(ref);
      if (el && isOffscreen(el)) offscreen.push(ref);
    }

    const se = document.scrollingElement || document.documentElement;
    return {
      docId: state.docId,
      url: location.href,
      title: document.title,
      viewport: [window.innerWidth, window.innerHeight],
      scroll: [Math.round(se.scrollTop), Math.max(0, se.scrollHeight - window.innerHeight)],
      dpr: window.devicePixelRatio || 1,
      nodes,
      offscreen,
    };
  }

  function deepElementFromPoint(root, x, y) {
    let el = root.elementFromPoint(x, y);
    while (el && el.shadowRoot) {
      const inner = el.shadowRoot.elementFromPoint(x, y);
      if (!inner || inner === el) break;
      el = inner;
    }
    return el;
  }

  function composedContains(parent, child) {
    for (let n = child; n; n = n.parentNode || n.host) {
      if (n === parent) return true;
    }
    return false;
  }

  // Diz o que está por cima de um elemento. Um backdrop sem papel costuma conter o dialog: reporta o dialog.
  function describe(el) {
    const dialogSel = 'dialog,[role=dialog],[role=alertdialog],[aria-modal=true]';
    for (const cand of [el.closest?.(dialogSel), el.querySelector?.(dialogSel)]) {
      const ref = cand?.getAttribute(REF_ATTR);
      if (ref && find(ref) === cand) return { ref, role: roleOf(cand) };
    }
    for (let n = el; n; n = n.parentElement || n.getRootNode()?.host) {
      const ref = n.getAttribute?.(REF_ATTR);
      if (ref && find(ref) === n) return { ref, role: roleOf(n) || n.tagName.toLowerCase() };
    }
    const cls = typeof el.className === 'string' && el.className ? '.' + el.className.trim().split(/\s+/)[0] : '';
    return { role: el.tagName.toLowerCase() + cls };
  }

  // Prepara o clique: rola até o elemento e devolve o ponto (coordenadas da página principal).
  function hit(ref) {
    const el = find(ref);
    if (!el) return { error: 'not_found' };
    const role = roleOf(el);
    const info = { role, tag: el.tagName.toLowerCase(), password: el.tagName === 'INPUT' && el.type === 'password' };
    try { info.disabled = el.matches(':disabled') || el.getAttribute('aria-disabled') === 'true'; } catch (_) {}
    if (!isVisible(el)) return { ...info, error: 'hidden' };
    el.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' });
    const r = el.getBoundingClientRect();
    if (r.width <= 0 || r.height <= 0) return { ...info, error: 'hidden' };
    const win = el.ownerDocument.defaultView;
    const left = Math.max(r.left, 0), right = Math.min(r.right, win.innerWidth);
    const top = Math.max(r.top, 0), bottom = Math.min(r.bottom, win.innerHeight);
    if (right <= left || bottom <= top) return { ...info, error: 'offscreen' };
    const cx = (left + right) / 2, cy = (top + bottom) / 2;
    const at = deepElementFromPoint(el.ownerDocument, cx, cy);
    const own = at && (composedContains(el, at) || (at.tagName === 'LABEL' && at.control === el));
    if (at && !own) info.coveredBy = describe(at);
    const off = frameOffset(el.ownerDocument);
    info.x = cx + off.x;
    info.y = cy + off.y;
    info.box = boxOf(el);
    info.kind = cursorKind(el);
    return info;
  }

  // Cursor visual (docs/CURSOR.md, 12.2). Um ponto só vale se acertar o alvo ou um descendente NÃO
  // interativo: num card clicável com uma lixeira dentro, um ponto na lixeira não é clique no card.
  function isInteractive(n) {
    if (!n || n.nodeType !== 1) return false;
    if (n.matches('a[href],button,input,select,textarea,[onclick]')) return true;
    const ce = n.getAttribute('contenteditable');
    if (ce !== null && ce !== 'false') return true;
    const ti = n.getAttribute('tabindex');
    if (ti !== null && Number(ti) >= 0) return true;
    const role = n.getAttribute('role');
    return !!role && INTERACTIVE_ROLES.has(role);
  }

  // hitsOnly: (x, y) em coordenadas da página principal acerta el (ou o label dele) sem passar por um
  // filho interativo.
  function hitsOnly(el, x, y) {
    const off = frameOffset(el.ownerDocument);
    const at = deepElementFromPoint(el.ownerDocument, x - off.x, y - off.y);
    if (!at) return false;
    if (at.tagName === 'LABEL' && at.control === el) return true;
    if (!composedContains(el, at)) return false;
    for (let n = at; n && n !== el; n = n.parentNode || n.host) {
      if (isInteractive(n)) return false;
    }
    return true;
  }

  function cursorKind(el) {
    if (editableInfo(el)) return 'text';
    const c = getComputedStyle(el).cursor;
    return c === 'text' || c === 'pointer' ? c : 'arrow';
  }

  // aim valida os candidatos sorteados pelo Go numa ida só e devolve o primeiro que vale. Se nenhum
  // vale, devolve os retângulos dos filhos interativos para o Go achar a maior área livre. Também
  // conta o que muda o plano: aba oculta e prefers-reduced-motion.
  function aim(ref, cands) {
    const el = find(ref);
    if (!el) return { error: 'not_found' };
    const index = (cands || []).findIndex((c) => hitsOnly(el, c[0], c[1]));
    const obstacles = [];
    if (index < 0) {
      for (const n of el.querySelectorAll('*')) {
        if (isInteractive(n) && isVisible(n)) obstacles.push(boxOf(n));
        if (obstacles.length >= 64) break;
      }
    }
    return { box: boxOf(el), kind: cursorKind(el), index, obstacles, ...env() };
  }

  // env é o que muda o plano do cursor: aba oculta, movimento reduzido e o tamanho da viewport.
  function env() {
    return {
      hidden: document.visibilityState !== 'visible',
      reduced: !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches),
      viewport: [window.innerWidth, window.innerHeight],
    };
  }

  function aimAt(ref, x, y) {
    const el = find(ref);
    return !!el && hitsOnly(el, x, y);
  }

  // revealSmooth começa uma rolagem suave até o alvo, se ele não estiver inteiro na tela (5.1).
  function revealSmooth(ref) {
    const el = find(ref);
    if (!el) return { error: 'not_found' };
    const r = el.getBoundingClientRect();
    const win = el.ownerDocument.defaultView;
    const off = r.top < 0 || r.left < 0 || r.bottom > win.innerHeight || r.right > win.innerWidth;
    if (off) el.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' });
    return { scrolling: off };
  }

  // Rola até o elemento se ele não estiver inteiro na tela e devolve a caixa (para o destaque).
  function reveal(ref) {
    const el = find(ref);
    if (!el) return { error: 'not_found' };
    const r = el.getBoundingClientRect();
    const win = el.ownerDocument.defaultView;
    if (r.top < 0 || r.left < 0 || r.bottom > win.innerHeight || r.right > win.innerWidth) {
      el.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'instant' });
    }
    return { box: boxOf(el), kind: cursorKind(el) };
  }

  function editableInfo(el) {
    if (el.tagName === 'TEXTAREA') return true;
    if (el.tagName === 'INPUT') return TEXT_INPUTS.has(el.type) || ['search', 'number'].includes(el.type);
    return el.isContentEditable;
  }

  // Foca e (opcionalmente) seleciona todo o conteúdo para que o Backspace real o apague.
  function prepareType(ref, clear) {
    const el = find(ref);
    if (!el) return { error: 'not_found' };
    const info = { role: roleOf(el), password: el.tagName === 'INPUT' && el.type === 'password' };
    try { info.disabled = el.matches(':disabled') || el.getAttribute('aria-disabled') === 'true'; } catch (_) {}
    info.readonly = !!el.readOnly;
    info.editable = editableInfo(el);
    if (!info.editable || info.disabled || info.readonly) return info;
    el.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'instant' });
    info.box = boxOf(el);
    // Campo de senha: só rola até ele (para o destaque); quem digita é a pessoa.
    if (info.password) return info;
    el.focus();
    if (!clear) {
      if (el.setSelectionRange && el.value != null) {
        try { el.setSelectionRange(el.value.length, el.value.length); } catch (_) {}
      }
      return info;
    }
    if (el.select) {
      el.select();
      info.hasContent = el.value.length > 0;
      return info;
    }
    const range = el.ownerDocument.createRange();
    range.selectNodeContents(el);
    const sel = el.ownerDocument.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
    info.hasContent = norm(el.innerText).length > 0;
    return info;
  }

  function selectOption(ref, option) {
    const el = find(ref);
    if (!el) return { error: 'not_found' };
    if (el.tagName !== 'SELECT') return { error: 'not_select', role: roleOf(el) };
    if (el.disabled) return { error: 'disabled' };
    const wanted = norm(option);
    const opts = [...el.options];
    const match = opts.find((o) => norm(o.label) === wanted) || opts.find((o) => o.value === option) ||
      opts.find((o) => norm(o.label).toLowerCase() === wanted.toLowerCase());
    if (!match) return { error: 'no_option', available: opts.map((o) => norm(o.label)).slice(0, 30) };
    if (match.disabled) return { error: 'option_disabled', label: norm(match.label) };
    el.focus();
    match.selected = true;
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
    return { label: norm(match.label) };
  }

  // Raízes de busca: documento, shadow roots abertos e iframes same-origin.
  function searchRoots() {
    const roots = [];
    const visit = (root) => {
      roots.push(root);
      const walker = (root.ownerDocument || root).createTreeWalker(root, NodeFilter.SHOW_ELEMENT);
      for (let n = walker.nextNode(); n; n = walker.nextNode()) {
        if (n.shadowRoot) visit(n.shadowRoot);
        if (n.tagName === 'IFRAME') {
          try { if (n.contentDocument?.body) visit(n.contentDocument.body); } catch (_) {}
        }
      }
    };
    visit(document.body || document.documentElement);
    return roots;
  }

  function visibleText(el) {
    return norm(el.innerText).toLowerCase();
  }

  // Desce enquanto algum filho visível ainda contém o texto: o resultado é o elemento mais próximo.
  function findText(text) {
    const needle = norm(text).toLowerCase();
    if (!needle) return { found: false };
    let count = 0;
    let first = null;
    for (const root of searchRoots()) {
      const starts = root.nodeType === Node.DOCUMENT_FRAGMENT_NODE ? [...root.children] : [root];
      for (const start of starts) {
        if (!isRendered(start) || !visibleText(start).includes(needle)) continue;
        let el = start;
        for (;;) {
          const next = [...el.children].find((c) => isRendered(c) && isVisible(c) && visibleText(c).includes(needle));
          if (!next) break;
          el = next;
        }
        if (!isVisible(el)) continue;
        const r = el.getBoundingClientRect();
        if (r.width <= 0 || r.height <= 0) continue;
        count++;
        if (!first) first = el;
      }
    }
    if (!first) return { found: false };
    return { found: true, ref: refFor(first), role: roleOf(first) || first.tagName.toLowerCase(), text: cut(norm(first.innerText)), count };
  }

  function quietMs() {
    let last = state.lastMutation;
    for (const f of document.querySelectorAll('iframe')) {
      try {
        const m = f.contentWindow?.__vitral;
        if (m) last = Math.max(last, m.lastMutationAt() - performance.timeOrigin + f.contentWindow.performance.timeOrigin);
      } catch (_) {}
    }
    return performance.now() - last;
  }


  const STYLE_PROPS = [
    'color', 'background-color', 'background-image', 'font-family', 'font-size', 'font-weight', 'font-style',
    'line-height', 'text-align', 'text-decoration-line', 'text-transform', 'border-top-width', 'border-top-style',
    'border-top-color', 'border-right-width', 'border-bottom-width', 'border-left-width', 'border-radius',
    'padding', 'margin', 'opacity', 'cursor', 'display', 'visibility', 'box-shadow', 'outline', 'z-index', 'position',
  ];
  const ATTRS = ['id', 'class', 'type', 'name', 'href', 'placeholder', 'title', 'tabindex', 'for', 'value'];

  function effectiveBackground(el) {
    for (let n = el; n && n.nodeType === Node.ELEMENT_NODE; n = n.parentElement || n.getRootNode()?.host) {
      const bg = getComputedStyle(n).backgroundColor;
      if (bg && bg !== 'transparent' && !/rgba\(.*,\s*0\)$/.test(bg)) {
        const ref = n.getAttribute(REF_ATTR);
        return { color: bg, source: ref && find(ref) === n ? ref : n.tagName.toLowerCase() };
      }
    }
    return { color: 'rgb(255, 255, 255)', source: 'default' };
  }

  // Estilos computados e atributos para a ferramenta inspect.
  function inspect(ref, props) {
    const el = find(ref);
    if (!el) return { error: 'not_found' };
    const st = getComputedStyle(el);
    const styles = {};
    for (const p of STYLE_PROPS) styles[p] = st.getPropertyValue(p);
    const extra = {};
    for (const p of props || []) extra[p] = st.getPropertyValue(p);
    const aria = {};
    const attrs = {};
    for (const a of el.attributes) {
      if (a.name.startsWith('aria-') || a.name === 'role') aria[a.name] = a.value;
      else if (ATTRS.includes(a.name)) attrs[a.name] = a.name === 'value' && el.type === 'password' ? '••••' : a.value;
    }
    const role = roleOf(el) || el.tagName.toLowerCase();
    return {
      ref, role, name: nameOf(el, role), tag: el.tagName.toLowerCase(), box: boxOf(el),
      styles, extra, aria, attrs, states: statesOf(el, role), background: effectiveBackground(el),
      text: cut(norm(el.innerText || '')).slice(0, 120),
    };
  }

  window.__vitral = {
    snapshot,
    find,
    hit,
    prepareType,
    selectOption,
    findText,
    inspect,
    box: (ref) => { const el = find(ref); return el ? boxOf(el) : null; },
    reveal,
    revealSmooth,
    aim,
    aimAt,
    env,
    quietMs,
    roleOf,
    lastMutationAt: () => state.lastMutation,
    docId: () => state.docId,
  };
  window.__vitralSnapshot = snapshot;
})();
