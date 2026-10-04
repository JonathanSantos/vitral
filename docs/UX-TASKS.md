# Vitral: tarefas de experiência de uso

> Adendo à spec principal (`docs/SPEC.md`). Cores, fontes e tom de voz vêm de `docs/BRAND.md`.
> UX-1 a UX-3 formam uma fase nova; UX-4 a UX-6 complementam a Fase 6 (experiência).

O público inclui pessoas não técnicas. O objetivo é que elas **vejam o que a IA está fazendo, confiem e possam assumir o controle quando quiserem**.

---

## Regras comuns aos overlays na página

Valem para o destaque (UX-1), o selo (UX-2) e a pausa (UX-3):

- Tudo é injetado via `EvalOnNewDocument` dentro de **um único host** (`<vitral-overlay>`) com **shadow root fechado**, anexado ao `documentElement`.
- O host fica com `position: fixed`, `z-index` máximo e `pointer-events: none`. Só os controles clicáveis do selo têm `pointer-events: auto`.
- **Invisível para a IA:** o snapshot ignora `<vitral-overlay>` e tudo dentro dele; o `screenshot` esconde o host durante a captura e o restaura depois. Os dois casos têm teste.
- **Não interfere na página:** não carrega nada da rede; não aparece no histórico de rede nem no console; não altera layout, foco ou scroll.
- Cores e fontes vêm de `internal/brand`, passadas como custom properties.
- O estado vive em Go e a página só renderiza. Depois de cada navegação, o overlay é reinjetado e reidratado com o estado atual.
- A comunicação página → Go usa uma binding do CDP (`Runtime.addBinding`) com nome prefixado (`__vitral_*`), validando o payload.
- `--no-overlay` desliga UX-1 e UX-2. A pausa (UX-3) continua disponível pelo selo mínimo.

## UX-1: mostrar o que a IA vai fazer

Antes de cada `click`, `type`, `select` e `scroll` com ref, o elemento alvo pisca na cor da marcação dele.

- **Destaque:** contorno de 3 px na cor `MarkColor(ref)`, raio de 6 px, desenhado no overlay a partir da bounding box, e etiqueta com a ref igual à do screenshot.
- **Tempo:** aparece, espera 350 ms (`--highlight-ms`, `0` desliga) e a ação acontece. Some em 400 ms com fade.
- **Duração total:** a ação não fica mais lenta que o destaque mais 50 ms.
- **Headless:** pula o destaque inteiro.
- **Aceite:** o clique acerta o alvo com o overlay ativo; o snapshot capturado durante o destaque não contém nada do overlay.

## UX-2: selo "Vitral está usando esta janela"

Pílula fixa no canto inferior direito, a 16 px das bordas, com o ícone pequeno (`icon-small.svg` inline), o texto de status e o botão de pausa.

| Estado | Texto |
|---|---|
| ocioso | "Vitral está usando esta janela" |
| trabalhando (ferramenta em execução) | "A IA está trabalhando…" |
| fluxo em execução | "Executando fluxo: {nome} · passo {i} de {n}" |
| pausado | "Pausado. Você está no controle." |
| aguardando o usuário (UX-3) | "Sua vez: {motivo}" |

- Pode ser recolhido para um botão circular só com o ícone (`aria-label`). O estado recolhido é por sessão, em Go.
- Não pode arrastar na v1.
- Texto com contraste ≥ 4,5:1 e alvos de toque ≥ 44 px.
- **Aceite:** aparece após `open`, sobrevive a navegação e reload, muda de estado durante uma ferramenta lenta; não aparece no screenshot nem no snapshot.

## UX-3: pausar e assumir o controle

- **Botão no selo:** "⏸ Pausar IA" alterna com "▶ Retomar". A página chama a binding; o Go muda o estado e devolve o novo estado para renderizar.
- **Enquanto pausado:**
  - Ferramentas que agem (`open`, `click`, `type`, `select`, `press`, `scroll`, `back`, `emulate`, `mock_*`, `flow_run`, `eval_js`) não executam e retornam um resultado **não-erro**: `Paused by the user: they are using the browser window. Do not retry now. Tell the user, in their language, to click "Retomar" in the browser when they are done, then call snapshot.`
  - Ferramentas de leitura (`snapshot`, `screenshot`, `inspect`, `network_*`, `console_list`, `flow_list`) continuam funcionando.
- **Retomada:** só pelo botão. Não existe ferramenta MCP para retomar.
- **Ao retomar:** o estado volta a ocioso e o próximo diff considera tudo o que mudou durante a pausa.
- **"Sua vez" automático:** quando o `type` mirar um campo de senha, o Vitral entra em pausa com "Sua vez: preencha a senha e clique em Retomar", o elemento fica destacado até o usuário retomar e a ferramenta devolve uma mensagem equivalente à da pausa.
- **Durante uma ferramenta em andamento:** a pausa vale a partir da próxima chamada. `flow_run` verifica a pausa entre os passos e para no passo seguinte, informando onde parou.
- **Testes:** unitário da máquina de estados (ocioso, trabalhando, pausado, aguardando) como função pura de transição; integração clicando no botão do selo e verificando o bloqueio das ações e a liberação das leituras.

## UX-4: CLI amigável para humanos

- **`vitral` sem argumentos:** com stdin em terminal, imprime uma ajuda curta em PT-BR e sai com 0; sem terminal (Copilot), roda o servidor MCP.
- **`vitral setup`:** cria ou atualiza `.mcp.json` (formato portável, `mcpServers`) com a entrada `vitral`, preservando as outras; copia os prompt files para `.github/prompts/` sem sobrescrever sem confirmar; imprime os próximos passos; `--global` grava `~/.copilot/mcp-config.json` (todos os perfis do VS Code e o Copilot CLI). Onde o Vitral estiver no formato antigo do VS Code (`.vscode/mcp.json`, `mcp.json` do perfil), oferece tirar. `vitral status`, `vitral disable` e `vitral enable` completam o ciclo (README, "Desligar o Vitral").
- **`vitral doctor`:** ✓/✗ por item com a ação sugerida: Chrome/Chromium encontrado, perfil gravável, launch headless funciona, versão.
- **`vitral version`.**
- **Download do Chromium:** progresso em stderr em PT-BR ("Baixando o navegador do Vitral (só na primeira vez)… 45%").
- Mensagens para humanos centralizadas em `internal/cli/messages.go`.
- **Aceite:** testes de `setup` em diretório temporário (merge do `mcp.json`, não sobrescrever) e da detecção TTY vs pipe.

## UX-5: prompts prontos com cara de produto

Em `examples/vscode/.github/prompts/`, no formato atual do VS Code, em PT-BR, cada um só com as ferramentas que precisa:

| Arquivo | O que faz |
|---|---|
| `vitral-descrever-tela.prompt.md` | abre uma URL e descreve a tela em linguagem simples |
| `vitral-preencher-formulario.prompt.md` | preenche um formulário com dados colados no chat e para antes de enviar |
| `vitral-testar-api-fora.prompt.md` | mocka erro 500 ou timeout e mostra (com print) como a tela se comporta |
| `vitral-executar-fluxo.prompt.md` | lista os fluxos, pede os parâmetros e executa |
| `vitral-gravar-fluxo.prompt.md` | guia a tarefa com a IA e salva como fluxo no fim |

Regras: a IA explica o que vai fazer antes de agir; nunca envia formulários nem confirma ações irreversíveis sem o "sim" da pessoa; lembra que dá para pausar pelo selo.

## UX-6: README e materiais

- README segue a seção 8 do BRAND.md, com "Comece em 3 passos" usando `vitral setup`.
- `assets/brand/` (ícones e PNGs), com o ícone no topo do README.
- `docs/demo.md` com o roteiro de um GIF de 20–30 s: abrir a página, a IA marcar as refs, o destaque antes do clique, a pausa e a retomada.

## Fora de escopo agora

Tradução para outros idiomas (só centralizar as mensagens), arrastar o selo, atalho de teclado global para pausar, sons.
