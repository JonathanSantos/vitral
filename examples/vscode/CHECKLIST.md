# Checklist de validação manual no VS Code (Copilot)

Pré-requisitos: binário `browser-mcp` no PATH (ou caminho absoluto em `command`) e o
`mcp.json` desta pasta copiado para `.vscode/mcp.json` do projeto de teste.

## Fase 1: base

1. Abra o projeto no VS Code e rode **MCP: List Servers**. O servidor `browser` deve aparecer.
   Clique em **Start**. O status vira "Running" sem abrir nenhuma janela de browser ainda.
2. Abra o chat do Copilot no modo **Agent** e confira em **Tools** (ícone de ferramentas) que aparecem
   `open`, `snapshot`, `click`, `type`, `select`, `press`, `scroll`, `wait_for` e `back`, e que **não**
   aparece `eval_js`.
3. Peça: *"Abra https://example.com e me diga o que tem na tela."*
   - Uma janela do Chrome abre (visível) e navega.
   - A resposta da ferramenta `open` mostra `page: "Example Domain"` e um `link` com ref `[eN]`.
4. Peça: *"Clique no link."* A resposta da ferramenta mostra só o diff (URL nova ou snapshot novo).
5. Peça: *"Volte para a página anterior."* (`back`).
6. Num site com formulário de login, peça para digitar a senha. A ferramenta `type` deve recusar com
   `password fields must be filled by the user in the browser window`.
7. Feche a aba do Chrome manualmente e peça um `snapshot`. Uma aba nova abre, sem erro.
8. Em **MCP: List Servers → browser → Show Output**, confira que não há erros de protocolo
   (logs do servidor aparecem como stderr).
9. Pare o servidor (**Stop**). A janela do Chrome lançada por ele fecha.
