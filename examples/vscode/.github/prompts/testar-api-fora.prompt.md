---
name: testar-api-fora
description: Testa como uma tela se comporta quando a API está fora do ar, lenta ou devolvendo erro
argument-hint: endereço da tela e, se souber, o caminho da API (ex.: /api/pedidos)
agent: agent
tools: ['browser/*']
---
Você vai testar a tela ${input:url:endereço da tela} simulando problemas na API, sem mexer em
nenhum servidor de verdade. Use as ferramentas do servidor `browser`.

1. Abra a tela com `open` e use `network_list` (filtro `type: fetch` ou `xhr`) para descobrir quais
   chamadas de API ela faz. Se a pessoa informou o caminho (${input:api:caminho da API, opcional}),
   use esse.
2. Para cada cenário abaixo, crie o mock com `mock_add`, refaça a ação que chama a API (recarregue com
   `open` ou clique no botão) e registre o que a tela mostra (use `screenshot` quando ajudar):
   - **erro do servidor**: `status: 500`;
   - **sem conexão**: `fail: connection_refused`;
   - **lentidão**: `delayMs: 5000`, para observar o indicador de carregamento; depois use `wait_for`
     até o conteúdo aparecer;
   - **lista vazia**: `status: 200` com um corpo JSON vazio no mesmo formato da resposta real
     (veja o formato com `network_get`).
3. Entre um cenário e outro, use `mock_clear`.
4. No fim, apague todos os mocks (`mock_clear`) e entregue um resumo em português:
   cenário → o que a pessoa vê → se está bom ou o que melhorar (mensagem clara? botão de tentar de
   novo? tela travada?).

Nunca digite senhas. Se precisar de login, peça para a pessoa entrar pela janela do navegador.
