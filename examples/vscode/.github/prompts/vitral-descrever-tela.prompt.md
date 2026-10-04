---
name: vitral-descrever-tela
description: Abre uma página no navegador e explica o que aparece na tela, em linguagem simples
argument-hint: endereço da página (ex.: https://portal.empresa.com/pedidos)
agent: agent
tools: ['vitral/open', 'vitral/snapshot', 'vitral/screenshot', 'vitral/scroll', 'vitral/console_list', 'vitral/network_list']
---
Siga a skill `vitral`.

Abra ${input:url:endereço da página} e descreva a tela em português simples: que página é, as principais
áreas, o que dá para fazer ali e as mensagens de erro ou aviso visíveis. Só olhe: não clique em nada que
salve, envie ou apague.
