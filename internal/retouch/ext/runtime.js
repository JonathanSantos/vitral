// Retoques do Vitral: o content script que aplica o CSS de cada retoque (docs/RETOQUES.md, 6.1).
// Roda em document_start, no mundo isolado da extensão, antes do primeiro script da página. Cada
// retoque é um <style> próprio, ligado e desligado pelo atributo media, sem recarregar a página.
(() => {
  if (globalThis.__vitralRetoques) return;
  const root = document.documentElement;
  const styles = new Map(); // nome → <style>
  let disabled = new Set(); // desligados no painel (chrome.storage, por perfil)
  let original = false; // "Ver original" nesta página

  let pending = false;
  const apply = () => {
    for (const [name, el] of styles) el.media = original || disabled.has(name) ? 'not all' : 'all';
    if (pending) return;
    pending = true;
    queueMicrotask(() => {
      pending = false;
      const active = original ? 0 : [...styles.keys()].filter((n) => !disabled.has(n)).length;
      chrome.runtime.sendMessage({ type: 'count', n: active }).catch(() => {});
    });
  };

  // Ligado é o padrão: o CSS entra na hora (sem piscar o original) e a lista de desligados, que chega
  // logo depois, só tira o que a pessoa desligou.
  chrome.storage.local.get('disabled').then((v) => { disabled = new Set(v.disabled || []); apply(); });
  chrome.storage.onChanged.addListener((c, area) => {
    if (area === 'local' && c.disabled) { disabled = new Set(c.disabled.newValue || []); apply(); }
  });

  // "Ver original" vem do painel (mensagem) ou do selo do Vitral, que roda em outro mundo isolado: o
  // ponto em comum é o atributo no <html>.
  const setOriginal = (on) => { original = on; if (root) root.toggleAttribute('data-vitral-original', on); apply(); };
  if (root) {
    new MutationObserver(() => {
      const on = root.hasAttribute('data-vitral-original');
      if (on !== original) { original = on; apply(); }
    }).observe(root, { attributes: true, attributeFilter: ['data-vitral-original'] });
  }
  chrome.runtime.onMessage.addListener((m, _sender, reply) => {
    if (m.type === 'status') reply({ names: [...styles.keys()], disabled: [...disabled], original });
    if (m.type === 'original') { setOriginal(!!m.on); reply({ original }); }
  });

  // Página que troca o <head> ou limpa o <html> (alguns frameworks): os <style> voltam.
  const keep = () => styles.forEach((el) => { if (!el.isConnected) (document.head || root).append(el); });
  if (root) new MutationObserver(keep).observe(root, { childList: true });
  document.addEventListener('DOMContentLoaded', keep);

  globalThis.__vitralRetoques = {
    add(name, css) {
      if (!root || styles.has(name)) return;
      const s = document.createElement('style');
      s.dataset.vitralRetoque = name;
      s.textContent = css;
      (document.head || root).append(s);
      styles.set(name, s);
      apply();
    },
  };
})();
