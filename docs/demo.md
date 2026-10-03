# Roteiro do GIF de demonstração (20–30 s)

Objetivo: mostrar, sem narração, que a IA enxerga a tela em pedaços, avisa antes de agir e que a pessoa
pode assumir o controle quando quiser.

## Preparação

- Janela do VS Code à esquerda (chat do Copilot no modo Agent) e janela do navegador do Vitral à direita,
  lado a lado, mesma altura. Resolução de gravação: 1600×900.
- Página de exemplo com busca, botão e tabela, como `testdata/pages/form.html` ou uma tela de pedidos
  com dados fictícios. Nada de dados reais.
- `vitral setup` já rodado, servidor `vitral` iniciado, selo visível no canto da janela do navegador.
- Destaque padrão (`--highlight-ms` 350). Se a gravação ficar rápida demais, use 600 só para o GIF.

## Cenas

| Tempo | Cena | O que aparece |
|---|---|---|
| 0–4 s | Pedido | No chat: *"Abra o portal de pedidos e me diga o que tem na tela."* O navegador abre a página; o selo mostra "A IA está trabalhando…". |
| 4–9 s | A IA enxerga em pedaços | A IA tira um print: aparece no chat a imagem com as marcações coloridas `e1`, `e2`, `e3`… nas cores do vitral. A resposta descreve a tela em uma frase. |
| 9–14 s | Destaque antes do clique | No chat: *"Busque o pedido 1023."* Na janela, o campo de busca pisca com contorno e etiqueta da ref, o texto é digitado, e o botão "Filtrar" pisca antes do clique. |
| 14–20 s | Pausa | A pessoa clica em "⏸ Pausar IA" no selo. O selo muda para "Pausado. Você está no controle." A pessoa rola a tabela e clica numa linha. No chat, um novo pedido da IA volta com "pausado, clique em Retomar". |
| 20–26 s | Retomada | A pessoa clica em "▶ Retomar". O selo volta para "Vitral está usando esta janela". A IA continua e responde considerando o que a pessoa mudou. |
| 26–30 s | Assinatura | Corte para o logotipo `vitral` com a tagline "Uma janela que a IA consegue ler." sobre fundo Vidro. |

## Acabamento

- Sem cursor piscando à toa: mova o mouse só quando a pessoa age (pausa e retomada).
- Legendas curtas (opcionais) em Inter: "A IA vê a tela em pedaços", "Ela avisa antes de clicar",
  "Você assume quando quiser".
- Exportar em GIF (≤ 8 MB para o README) e em MP4. Salvar como `assets/demo.gif` e trocar o comentário
  `TODO` do topo do README pela imagem.
