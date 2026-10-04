---
name: vitral-executar-fluxo
description: Lista os fluxos gravados, pede os dados que faltam e executa passo a passo
argument-hint: nome do fluxo e os valores (ex.: buscar-pedido numero=2048)
agent: agent
tools: ['vitral/flow_list', 'vitral/flow_run', 'vitral/snapshot', 'vitral/screenshot']
---
Siga a skill `vitral`.

Pedido: ${input:pedido:nome do fluxo e valores}. Mostre os fluxos com `flow_list`, confirme qual vai rodar
e com quais valores (pergunte os que faltarem) e rode com `flow_run`. Se o fluxo envia, salva ou apaga
algo, peça um "sim" antes. No fim, conte o que foi feito ou, se falhou, o passo e o motivo.
