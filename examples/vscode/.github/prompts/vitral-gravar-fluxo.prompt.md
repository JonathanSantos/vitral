---
name: vitral-gravar-fluxo
description: Faz uma tarefa no navegador junto com você e salva como fluxo para repetir depois
argument-hint: o que a tarefa faz (ex.: buscar um pedido pelo número)
agent: agent
tools: ['vitral/open', 'vitral/snapshot', 'vitral/click', 'vitral/type', 'vitral/select', 'vitral/press', 'vitral/scroll', 'vitral/wait_for', 'vitral/back', 'vitral/screenshot', 'vitral/flow_save', 'vitral/flow_list']
---
Você vai ajudar a pessoa a gravar uma tarefa repetitiva como fluxo do Vitral. A pessoa pode pausar
você pelo selo do Vitral, no canto da janela do navegador, e fazer uma parte ela mesma.

Tarefa: ${input:tarefa:o que a tarefa faz}

1. Combine o plano: pergunte em que endereço a tarefa começa e quais valores mudam a cada vez (ex.: o
   número do pedido). Esses valores viram parâmetros.
2. Faça a tarefa passo a passo, explicando antes de cada ação. Cada ação bem-sucedida termina com
   `step #N`: anote o número do primeiro passo.
3. Use `wait_for` no fim com um texto que prove que deu certo (ex.: o número do pedido na tela). Isso
   deixa o fluxo confiável.
4. Não envie, salve nem apague nada sem um "sim" da pessoa. Senhas são sempre digitadas por ela.
5. No fim, proponha um nome curto (minúsculas e hífens), uma descrição e os parâmetros, e chame
   `flow_save` com `fromStep` igual ao primeiro passo e `params` com os valores usados
   (ex.: `{numero: "1023"}`; cada valor precisa ter pelo menos 3 caracteres).
6. Mostre o resumo que o `flow_save` devolve (passos e substituições) e explique como repetir:
   "rode o fluxo X com o número Y", ou o prompt `/vitral-executar-fluxo`.
