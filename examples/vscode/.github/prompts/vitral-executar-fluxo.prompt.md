---
name: vitral-executar-fluxo
description: Lista os fluxos gravados, pede os dados que faltam e executa passo a passo
argument-hint: nome do fluxo e os valores (ex.: buscar-pedido numero=2048)
agent: agent
tools: ['vitral/flow_list', 'vitral/flow_run', 'vitral/snapshot', 'vitral/screenshot']
---
Você vai executar um fluxo gravado com o Vitral. O fluxo roda sozinho, sem improviso. A pessoa pode
pausar a qualquer momento pelo selo do Vitral, no canto da janela do navegador.

1. Use `flow_list` e mostre os fluxos disponíveis, com a descrição e os parâmetros de cada um.
2. O pedido da pessoa é: ${input:pedido:nome do fluxo e valores}. Escolha o fluxo e confirme em uma frase
   o que vai rodar e com quais valores. Se faltar algum parâmetro, pergunte antes.
3. Se o fluxo envia, salva ou apaga algo, peça um "sim" da pessoa antes de rodar.
4. Rode com `flow_run` passando `name` e `params`.
5. Se todos os passos derem `ok`, conte em português o que foi feito e o que a tela mostra no fim.
6. Se um passo falhar, o relatório diz qual passo, o motivo e mostra a tela. Explique em linguagem simples
   e sugira o próximo passo (por exemplo: "o botão agora se chama 'Confirmar'; quer que eu termine
   manualmente e grave o fluxo de novo?"). Se o fluxo parou porque a pessoa pausou, apenas avise.
