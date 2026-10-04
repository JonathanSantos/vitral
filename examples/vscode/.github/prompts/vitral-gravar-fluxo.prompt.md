---
name: vitral-gravar-fluxo
description: Faz uma tarefa no navegador junto com você e salva como fluxo para repetir depois
argument-hint: o que a tarefa faz (ex.: buscar um pedido pelo número)
agent: agent
tools: ['vitral/open', 'vitral/snapshot', 'vitral/click', 'vitral/type', 'vitral/select', 'vitral/press', 'vitral/scroll', 'vitral/wait_for', 'vitral/back', 'vitral/screenshot', 'vitral/flow_save', 'vitral/flow_list']
---
Siga a skill `vitral`.

Tarefa a gravar: ${input:tarefa:o que a tarefa faz}. Pergunte onde ela começa e quais valores mudam a cada
vez (viram parâmetros). Faça a tarefa passo a passo e salve com `flow_save` (com `expect`). Depois explique
como repetir: "rode o fluxo X com o valor Y" ou `/vitral-executar-fluxo`.
