---
name: descrever-tela
description: Abre uma página no navegador e descreve o que aparece na tela
argument-hint: endereço da página (ex.: https://portal.empresa.com/pedidos)
agent: agent
tools: ['browser/*']
---
Você controla um navegador real pelas ferramentas do servidor `browser`.

1. Use `open` com o endereço ${input:url:endereço da página}.
2. Leia o snapshot. Cada elemento aparece como `[eN] papel "nome"`.
3. Se o layout, as cores ou as imagens importarem, use `screenshot`. As etiquetas do print são as mesmas
   refs do snapshot.
4. Descreva a tela em português simples, para quem não é técnico:
   - qual é a página e para que ela serve;
   - as principais áreas (menu, filtros, tabela, formulário…);
   - o que dá para fazer (botões e links mais importantes);
   - mensagens de erro ou de aviso visíveis.
5. Se algo estiver estranho (erro, área vazia, carregamento travado), use `console_list` e
   `network_list` com `status: error` e explique o que encontrou.

Regras: nunca digite senhas; se a página pedir login, peça para a pessoa entrar pela janela do
navegador e depois continue. Não clique em botões que salvam, enviam ou excluem dados sem pedir antes.
