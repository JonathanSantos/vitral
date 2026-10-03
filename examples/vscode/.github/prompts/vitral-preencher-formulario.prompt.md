---
name: vitral-preencher-formulario
description: Preenche um formulário com os dados que você colar no chat e para antes de enviar
argument-hint: endereço do formulário (os dados você cola na conversa)
agent: agent
tools: ['vitral/open', 'vitral/snapshot', 'vitral/type', 'vitral/select', 'vitral/click', 'vitral/scroll', 'vitral/screenshot', 'vitral/wait_for']
---
Você vai preencher um formulário no navegador usando o Vitral. A pessoa pode pausar você a qualquer
momento pelo botão "⏸ Pausar IA" no selo do Vitral, no canto da janela.

Endereço do formulário: ${input:url:endereço do formulário}
Dados para preencher: ${input:dados:cole aqui os dados (nome, e-mail, endereço…)}

1. Abra o formulário com `open` e leia o snapshot.
2. Antes de digitar qualquer coisa, mostre um plano curto: campo do formulário → valor que vai usar.
   Aponte os campos obrigatórios que ficaram sem dado e pergunte o que fazer com eles.
3. Preencha com `type` (campos de texto), `select` (listas) e `click` (caixas de seleção e opções).
   Cada ação devolve só o que mudou: confira se o valor ficou certo e se apareceu algum erro de validação.
4. **Nunca digite senhas.** Se houver campo de senha, o Vitral passa a vez para a pessoa: peça para ela
   preencher na janela do navegador e clicar em "Retomar".
5. **Pare antes de enviar.** Não clique em "Enviar", "Salvar", "Confirmar" nem nada parecido. Mostre um
   resumo do que foi preenchido (use `screenshot` se ajudar) e pergunte: "Posso enviar?". Só clique
   depois de um "sim" claro da pessoa no chat.
6. Depois de enviar, use `wait_for` com a mensagem de sucesso ou de erro e conte o resultado.
