---
name: vitral-testar-api-fora
description: Mostra como a tela se comporta quando a API dá erro, demora ou fica fora do ar
argument-hint: endereço da tela (e, se souber, o caminho da API, ex.: /api/pedidos)
agent: agent
tools: ['vitral/open', 'vitral/snapshot', 'vitral/click', 'vitral/wait_for', 'vitral/screenshot', 'vitral/network_list', 'vitral/network_get', 'vitral/mock_add', 'vitral/mock_list', 'vitral/mock_clear']
---
Você vai simular problemas na API da tela ${input:url:endereço da tela}, sem mexer em nenhum servidor
de verdade: o Vitral só finge as respostas dentro deste navegador. A pessoa pode pausar você pelo selo
do Vitral, no canto da janela.

1. Explique em uma frase o que vai testar.
2. Abra a tela com `open` e use `network_list` (com `type: fetch` ou `xhr`) para descobrir as chamadas de
   API. Se a pessoa informou o caminho (${input:api:caminho da API, opcional}), use esse.
3. Para cada cenário: crie o mock com `mock_add`, refaça a ação que chama a API (`open` de novo ou
   `click` no botão) e tire um `screenshot` mostrando o resultado:
   - **erro no servidor**: `status: 500`;
   - **demora demais**: `fail: timeout`;
   - **lentidão**: `delayMs: 5000`, para mostrar o carregamento; depois `wait_for` até o conteúdo aparecer.
4. Entre um cenário e outro, use `mock_clear`. No fim, rode `mock_clear` de novo para não deixar nada
   simulado.
5. Entregue um resumo em português: cenário → o que a pessoa vê → está bom ou o que melhorar (a mensagem
   é clara? tem botão de tentar de novo? a tela trava?).

Não clique em nada que salve, envie ou apague dados sem perguntar antes.
