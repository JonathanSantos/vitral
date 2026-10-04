// Rolagem humana (pedido "próximos passos", seção 1.3). Só toca o plano calculado pelo Go
// (internal/cursor/scroll.go): deslocamentos por frame aplicados com scrollTo({behavior: 'instant'}),
// que não briga com o scroll-behavior: smooth da página. Usado pelo overlay e pelo tools/cursorlab.

// vitralScroll toca plan.f ([t, deslocamento, ...]) em el. done(result) recebe 'ok', 'hidden' (aba
// oculta: aplicado na hora) ou 'unresponsive' (dois frames sem efeito: o container não aceita rolagem
// programática e o Go cai para a roda do mouse).
function vitralScroll(el, plan, done) {
  const f = plan.f;
  const base = el.scrollTop;
  const total = f[f.length - 1];
  const apply = (off) => el.scrollTo({ top: base + off, behavior: 'instant' });
  let finished = false;
  const finish = (result) => {
    if (finished) return;
    finished = true;
    document.removeEventListener('visibilitychange', onHide);
    done && done(result);
  };
  const onHide = () => {
    if (document.visibilityState !== 'visible') {
      apply(total);
      finish('hidden');
    }
  };
  if (document.visibilityState !== 'visible' || f.length <= 2) {
    apply(total);
    return finish(document.visibilityState !== 'visible' ? 'hidden' : 'ok');
  }
  document.addEventListener('visibilitychange', onHide);
  let start = 0, i = 0, frames = 0;
  const step = (now) => {
    if (finished) return;
    if (!start) start = now;
    const t = now - start;
    while (i + 2 < f.length && f[i + 2] <= t) i += 2;
    let off = f[i + 1];
    if (i + 2 < f.length) {
      const k = (t - f[i]) / (f[i + 2] - f[i]);
      off = f[i + 1] + (f[i + 3] - f[i + 1]) * Math.min(Math.max(k, 0), 1);
    }
    apply(off);
    frames++;
    // Dois frames pedindo movimento sem a posição mudar: rolagem programática não funciona aqui.
    if (frames === 3 && Math.abs(off) >= 1 && el.scrollTop === base) return finish('unresponsive');
    if (t >= f[f.length - 2]) {
      apply(total);
      return finish('ok');
    }
    requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}
