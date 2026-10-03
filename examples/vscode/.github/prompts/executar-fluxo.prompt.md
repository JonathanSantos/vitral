---
name: executar-fluxo
description: Executa um fluxo salvo (passo a passo, sem improvisar) e explica o resultado
argument-hint: nome do fluxo e os valores dos parâmetros (ex.: buscar-pedido numero=2048)
agent: agent
tools: ['vitral/*']
---
Execute um fluxo gravado usando as ferramentas do servidor `vitral`.

1. Use `flow_list` para ver os fluxos disponíveis e os parâmetros de cada um.
2. O pedido da pessoa é: ${input:pedido:nome do fluxo e valores}. Escolha o fluxo certo. Se faltar
   algum parâmetro, pergunte antes de rodar.
3. Rode com `flow_run` passando `name` e `params`.
4. Se todos os passos passarem, conte em português o que foi feito e o que a tela mostra no fim.
5. Se um passo falhar, o relatório diz qual passo, o motivo e mostra a tela atual. Explique isso em
   linguagem simples e sugira o que fazer: por exemplo, se um botão mudou de nome, mostre qual nome
   aparece agora e pergunte se a pessoa quer que você termine manualmente e salve o fluxo atualizado
   com `flow_save`.

Não invente passos que não estão no fluxo sem perguntar. Nunca digite senhas.
