// Retoques do Vitral: o número no ícone é o de retoques ligados na aba.
chrome.action.setBadgeBackgroundColor({ color: '#1E1B2E' });
chrome.runtime.onMessage.addListener((m, sender) => {
  if (m.type === 'count' && sender.tab && sender.frameId === 0) {
    chrome.action.setBadgeText({ tabId: sender.tab.id, text: m.n ? String(m.n) : '' });
  }
});
