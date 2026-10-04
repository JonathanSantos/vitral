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
- Cursor da IA em `slow` só para a gravação: `"--cursor-speed", "slow"` em `args` no `mcp.json`.

## Cenas

| Tempo | Cena | O que aparece |
|---|---|---|
| 0–4 s | Pedido | No chat: *"Abra o portal de pedidos e me diga o que tem na tela."* O navegador abre a página; o selo mostra "A IA está trabalhando…". |
| 4–9 s | A IA enxerga em pedaços | A IA tira um print: aparece no chat a imagem com as marcações coloridas `e1`, `e2`, `e3`… nas cores do vitral. A resposta descreve a tela em uma frase. |
| 9–14 s | Destaque e cursor | No chat: *"Busque o pedido 1023."* Na janela, o cursor "IA" surge do selo e vai até o campo de busca, que pisca com contorno e etiqueta da ref; o texto aparece tecla a tecla; o cursor vai até "Filtrar" e o anel marca o clique. |
| 14–20 s | Pausa | A pessoa clica em "⏸ Pausar IA" no selo. O selo muda para "Pausado. Você está no controle." A pessoa rola a tabela e clica numa linha. No chat, um novo pedido da IA volta com "pausado, clique em Retomar". |
| 20–26 s | Retomada | A pessoa clica em "▶ Retomar". O selo volta para "Vitral está usando esta janela". A IA continua e responde considerando o que a pessoa mudou. |
| 26–30 s | Assinatura | Corte para o logotipo `vitral` com a tagline "Uma janela que a IA consegue ler." sobre fundo Vidro. |

## Acabamento

- O cursor que se move sozinho é o da IA (etiqueta "IA"). Mova o seu mouse só quando a pessoa age
  (pausa e retomada), para os dois não se confundirem na gravação.
- Legendas curtas (opcionais) em Inter: "A IA vê a tela em pedaços", "Ela avisa antes de clicar",
  "Você assume quando quiser".
- Exportar em GIF (≤ 8 MB para o README) e em MP4. Salvar como `assets/demo.gif` e trocar o comentário
  `TODO` do topo do README pela imagem.

## Só o cursor (`make demo-cursor`)

Para avaliar o movimento ou gravar um GIF curto só do cursor, sem o VS Code:

```bash
make demo-cursor
```

Abre uma janela visível do navegador do Vitral na `testdata/pages/cursor-playground.html` e roda, em
`slow`, este roteiro (cerca de 30 s), com uma pausa curta entre as ações:

1. Clique no botão grande "Continuar" (o cursor surge do selo).
2. Clique na caixa "Aceito os termos" (alvo pequeno: dá para ver a correção no fim do movimento).
3. Clique no link "política de privacidade", no meio do texto.
4. Digite "Olá! Aqui é o Vitral." no campo (cursor "I", afasta a mão, texto tecla a tecla).
5. Escolha "Porto Alegre" na cidade ("Selecionado: …" ao lado do cursor).
6. Abra o card "Pedido 1042", que tem um botão "Excluir" dentro (o clique nunca cai no excluir).
7. Clique em "Salvar" no botão dividido (o clique nunca cai na seta).
8. Tecle Tab (keycap "Tab" acima do selo).
9. Clique em "Voltar ao topo", lá embaixo (rolagem suave antes do movimento) e role para cima.

Grave a janela do navegador a 1280×800. A semente fixa repete exatamente os mesmos movimentos; para
outra trajetória, troque a semente:

```bash
make demo-cursor CURSOR_SEED=2
```
