// Painel dos Retoques (docs/RETOQUES.md, 3.1): os retoques desta aba, cada um com um interruptor,
// "Ver original" e "Desligar todos neste site". O estado fica no chrome.storage do perfil.
(async () => {
  const $ = (id) => document.getElementById(id);
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  let status = null;
  try { status = await chrome.tabs.sendMessage(tab.id, { type: 'status' }); } catch (_) { /* sem retoques nesta aba */ }
  if (!status || !status.names.length) { $('empty').hidden = false; return; }
  const about = await fetch(chrome.runtime.getURL('retoques.json')).then((r) => r.json()).catch(() => ({}));
  const getDisabled = async () => new Set((await chrome.storage.local.get('disabled')).disabled || []);
  const setDisabled = (set) => chrome.storage.local.set({ disabled: [...set] });

  const disabled = await getDisabled();
  for (const name of status.names) {
    const li = document.createElement('li');
    const box = document.createElement('input');
    box.type = 'checkbox';
    box.id = 'r-' + name;
    box.checked = !disabled.has(name);
    box.addEventListener('change', async () => {
      const set = await getDisabled();
      if (box.checked) set.delete(name); else set.add(name);
      await setDisabled(set);
    });
    const label = document.createElement('label');
    label.htmlFor = box.id;
    label.textContent = (about[name] && about[name].title) || name;
    if (about[name] && about[name].description) {
      const small = document.createElement('small');
      small.textContent = about[name].description;
      label.append(small);
    }
    li.append(box, label);
    $('list').append(li);
  }
  $('actions').hidden = false;

  let original = status.original;
  const paint = () => { $('original').textContent = original ? 'Ver retocado' : 'Ver original'; };
  paint();
  $('original').addEventListener('click', async () => {
    original = !original;
    await chrome.tabs.sendMessage(tab.id, { type: 'original', on: original });
    paint();
  });
  $('off').addEventListener('click', async () => {
    const set = await getDisabled();
    status.names.forEach((n) => set.add(n));
    await setDisabled(set);
    document.querySelectorAll('input[type=checkbox]').forEach((b) => { b.checked = false; });
  });
})();
