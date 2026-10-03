---
name: vitral-descrever-tela
description: Abre uma página no navegador e explica o que aparece na tela, em linguagem simples
argument-hint: endereço da página (ex.: https://portal.empresa.com/pedidos)
agent: agent
tools: ['vitral/open', 'vitral/snapshot', 'vitral/screenshot', 'vitral/scroll', 'vitral/console_list', 'vitral/network_list']
---
Você usa o Vitral para ver um navegador de verdade. A pessoa pode pausar você a qualquer momento pelo
selo "Vitral está usando esta janela", no canto da janela do navegador.

1. Diga em uma frase o que vai fazer: abrir ${input:url:endereço da página} e descrever a tela.
2. Use `open` com esse endereço. Leia o snapshot: cada elemento aparece como `[eN] papel "nome"`.
3. Se layout, cores ou imagens importarem, use `screenshot`. As etiquetas coloridas do print são as
   mesmas refs do snapshot. Use `scroll` se a página for longa.
4. Descreva em português simples, sem termos técnicos:
   - que página é e para que serve;
   - as principais áreas (menu, filtros, tabela, formulário…);
   - o que dá para fazer ali (os botões e links mais importantes);
   - mensagens de erro ou aviso visíveis.
5. Se algo parecer quebrado (erro, área vazia, carregando para sempre), confira `console_list` e
   `network_list` com `status: error` e explique o que encontrou.

Só olhe: não clique em nada que salve, envie ou apague. Se a página pedir login, peça para a pessoa
entrar pela janela do navegador e avisar quando terminar.
