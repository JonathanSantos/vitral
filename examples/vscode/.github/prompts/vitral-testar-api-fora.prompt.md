---
name: vitral-testar-api-fora
description: Mostra como a tela se comporta quando a API dá erro, demora ou fica fora do ar
argument-hint: endereço da tela (e, se souber, o caminho da API, ex.: /api/pedidos)
agent: agent
tools: ['vitral/open', 'vitral/snapshot', 'vitral/click', 'vitral/wait_for', 'vitral/screenshot', 'vitral/network_list', 'vitral/network_get', 'vitral/mock_add', 'vitral/mock_list', 'vitral/mock_clear']
---
Siga a skill `vitral`.

Tela: ${input:url:endereço da tela}. API (opcional): ${input:api:caminho da API, ex.: /api/pedidos}.
Simule três cenários, um de cada vez: erro 500, `fail: timeout` e lentidão (`delayMs: 5000`). Em cada
um, refaça a ação que chama a API e tire um `screenshot`. Entregue: cenário → o que a pessoa vê → está
bom ou o que melhorar.
