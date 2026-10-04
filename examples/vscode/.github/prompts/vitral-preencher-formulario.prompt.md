---
name: vitral-preencher-formulario
description: Preenche um formulário com os dados que você colar no chat e para antes de enviar
argument-hint: endereço do formulário (os dados você cola na conversa)
agent: agent
tools: ['vitral/open', 'vitral/snapshot', 'vitral/type', 'vitral/select', 'vitral/click', 'vitral/scroll', 'vitral/screenshot', 'vitral/wait_for']
---
Siga a skill `vitral`.

Formulário: ${input:url:endereço do formulário}
Dados: ${input:dados:cole aqui os dados (nome, e-mail, endereço…)}

Antes de digitar, mostre o plano campo → valor e pergunte sobre os obrigatórios sem dado. Preencha,
confira erros de validação e **pare antes de enviar**: mostre o resumo e pergunte "Posso enviar?".
