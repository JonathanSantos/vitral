# Spec: MCP local de controle de browser em Go

> Versão consolidada da spec original, já com as decisões tomadas na revisão do plano
> (ver "Decisões da revisão" no fim). Onde houver conflito, esta versão vale.

## 1. Contexto e objetivo

Servidor MCP local, distribuído como **binário único em Go**, que permite a uma IA leve no
**GitHub Copilot (VS Code)** controlar um browser Chromium. A IA deve conseguir:

- entender a tela por um **snapshot textual compacto**, com refs clicáveis;
- ver **prints com marcações** que batem com essas refs;
- inspecionar estilos (cores, fontes, estados) sob demanda;
- consultar o **histórico de rede** e o **console**;
- **mockar chamadas** de API;
- gravar fluxos que deram certo e **reexecutá-los** de forma determinística.

O público inclui pessoas não técnicas: instalação trivial e uso por fluxos prontos (prompt files).

Nome: **Vitral** (binário `vitral`), centralizado na constante `internal/config.Name`. Identidade em `docs/BRAND.md`.

## 2. Escopo

**v1 (dentro):** Chromium, uma aba ativa, snapshot com refs, ações básicas, screenshot com
marcações, inspeção de estilos, rede (lista e detalhe), mock, console, emulação simples,
gravação e replay de fluxos, transporte stdio.

**Fora da v1:** Firefox e WebKit, múltiplas abas simultâneas (estrutura preparada), transporte
HTTP, UI própria, iframes cross-origin, upload e download de arquivos.

## 3. Stack

| Item | Escolha |
|---|---|
| Linguagem | Go (versão estável mais recente) |
| MCP | `github.com/modelcontextprotocol/go-sdk`, transporte stdio |
| Browser | `github.com/go-rod/rod`; `proto` (CDP cru) onde o rod não cobrir |
| Desenho do print | `image`, `image/draw`, `golang.org/x/image/font/basicfont` |
| Config | flags de CLI e arquivo opcional `vitral.yaml` |
| YAML | `gopkg.in/yaml.v3` |

## 4. Padrões de código

- Clean code e SOLID, sem overengineering.
- Estilo funcional: funções puras sempre que possível; structs com métodos só onde há estado real.
- Early return e guard clauses; nada de `else` depois de `return`.
- Identificadores e mensagens técnicas em inglês; comentários em português, só explicando o porquê.
- Handlers nunca usam `Must*` do rod nem `panic`; erros viram `CallToolResult` com `IsError: true`.
- stdout é exclusivo do protocolo MCP; todo log vai para stderr (`log/slog`).
- Ações no browser são serializadas com um mutex por página.

## 5. Estrutura

```
cmd/vitral/main.go
internal/config/      flags + yaml, defaults, nome do app
internal/browser/     launch/attach, página ativa, reabertura de aba
internal/session/     sessão da página: mutex, ações, estabilidade, último snapshot
internal/snapshot/    JS embutido, parse, formatação, diff
internal/screenshot/  captura + marcações
internal/network/     ring buffer de requests, body sob demanda
internal/mock/        regras, matcher, Fetch
internal/console/     buffer de console/erros
internal/flow/        gravação, YAML, replay
internal/ring/        ring buffer genérico
internal/tools/       ferramentas MCP (um arquivo por grupo)
internal/security/    allowlist de hosts, mascaramento
testdata/pages/       fixtures HTML
examples/vscode/      .mcp.json, CHECKLIST.md
```

## 6. Browser

Modos: `launch` (padrão, visível, perfil persistente em `~/.vitral/profile`), `headless` e `attach`
(`--attach ws://...` ou `--attach-port 9222`; o Chrome precisa de `--user-data-dir` separado).

Navegador (o do usuário nunca é usado sem opt-in):

1. `--browser-path` / `VITRAL_BROWSER_PATH`;
2. Chrome for Testing gerenciado, versão fixada, em `~/.vitral/browsers/<versão>` (download com progresso
   em PT-BR, extração em pasta temporária, validação com `--version`, rename atômico; mantém a versão
   atual e a anterior). Plataformas: as que o JSON oficial lista para a versão (hoje linux64, linux-arm64,
   mac-arm64, mac-x64, win64);
3. se o download falhar ou não houver build: Chromium dos caches do Puppeteer/Playwright (o mais novo que
   roda; Firefox e WebKit ignorados);
4. Chrome/Edge do sistema só com `--allow-system-browser`, sempre com o perfil do Vitral;
5. senão, erro amigável em PT-BR com as saídas (mirror, zip manual, `--allow-system-browser`).

Redes corporativas: `VITRAL_BROWSER_MIRROR`/`--browser-mirror`, proxy do ambiente e
`vitral install-browser --from <zip>`.

Regras: subida preguiçosa; CDP só em `127.0.0.1`; ao encerrar fecha o browser lançado (nunca no
`attach`); se a aba for fechada, abre outra.

## 7. Snapshot

### 7.1 Geração

JS embutido (`go:embed`, `EvalOnNewDocument`) expõe `window.__vitralSnapshot(options)`. Percorre o DOM
(shadow roots abertos e iframes same-origin) e inclui elementos visíveis, interativos e estrutura
útil (headings, dialogs, tabelas, listas, landmarks, labels, alertas/erros). Cada elemento tem papel,
nome acessível, valor, estados e uma ref curta (`e1`, `e2`…) em `data-vitral-ref`, estável enquanto o
elemento existir e nunca reaproveitada na mesma página. Navegação completa reinicia o contador.

### 7.2 Formato

```
page: "Pedidos - Portal Interno" (https://portal/pedidos)
viewport: 1280x800, scroll: 0/2400
[e1] textbox "Buscar pedido" value=""
[e2] button "Filtrar"
[e3] table "Pedidos" rows=24 (showing 5)
  [e4] row "#1023 | Aprovado | R$ 450,00"
    [e5] link "Ver detalhes"
[e6] dialog "Confirmar exclusão" open
  [e7] button "Cancelar"
  [e8] button "Excluir"
[e9] alert "Pedido salvo com sucesso"
```

Compactação: textos truncados em 80 caracteres; listas e tabelas mostram 5 linhas + total;
wrappers sem papel colapsados; limite de 300 linhas com `… N more elements (use snapshot with
scope=<ref>)`. Opções: `scope`, `full`, `maxLines`.

### 7.3 Diff depois de ações

Toda ação espera estabilidade e devolve só o que mudou: URL/título, elementos adicionados (`+`),
removidos do DOM (`-`), alterados (`~`), resumo dos que saíram da viewport
(`- N elements left the viewport`, sem listar), erros novos de console e requests >= 400
disparados pela ação. Se o diff ficar maior que o snapshot, devolve o snapshot. Ações gravadas
terminam com `step #N`.

### 7.4 Erros acionáveis

- `ref e9 not found: page changed since last snapshot. Call snapshot.`
- `element e4 is disabled.`
- `element e2 is covered by dialog e6. Close it first.`
- `navigation timeout after 15s. Page may still be loading; call snapshot.`

## 8. Ações

- Localização por `[data-vitral-ref]`. `click` é clique real via CDP no centro, depois de rolar e
  checar cobertura (`elementFromPoint`). `type` foca, limpa (padrão) e digita; `submit: true` dá
  Enter. `select` aceita label ou value.
- Estabilidade: rede ociosa por 500 ms **e** DOM sem mutações por 300 ms, ou timeout (10 s,
  configurável), quando devolve o diff com aviso.

## 9. Ferramentas MCP

`open`, `snapshot`, `click`, `type`, `select`, `press`, `scroll`, `wait_for`, `back`, `screenshot`,
`inspect`, `network_list`, `network_get`, `mock_add`, `mock_list`, `mock_clear`, `console_list`,
`emulate`, `eval_js` (só com `--allow-eval`), `flow_save`, `flow_list`, `flow_run`, `flow_update`, `session_save`, `goto`, `map_find`.

`wait_for` com `text` procura no texto visível real do DOM (innerText dos elementos visíveis,
incluindo shadow roots abertos e iframes same-origin), não no snapshot, e devolve a ref do elemento
mais próximo que contém o texto.

## 10. Rede, mock e console

### 10.1 Histórico de rede

Domínio `Network` em ring buffer (500, limpo a cada navegação completa, flag para preservar).
Corpo sob demanda (`Network.getResponseBody`). Headers sensíveis (`Authorization`, `Cookie`,
`Set-Cookie` e configuráveis) mascarados.

### 10.2 Mock

- Regras em memória. Na avaliação, **a regra criada por último é testada primeiro**: a primeira que
  casar vence, ou seja, a mais recente tem prioridade.
- Matcher (glob de URL + método) é função pura testada. Padrão sem `*`/`?` casa como substring.
- Integração via domínio `Fetch`: `fulfillRequest`, `failRequest` e `delay` antes de responder.
- `times` limita aplicações; depois a regra expira.
- Requests mockadas aparecem no histórico como `mocked`.
- Flag para desabilitar o mock.

### 10.3 Console

`Runtime.consoleAPICalled`, `Runtime.exceptionThrown` e `Log.entryAdded` em ring buffer (200).

## 11. Segurança

`eval_js` só com `--allow-eval`; `--allowed-hosts` restringe navegação e mock; `type` recusa
`input[type=password]` (`password fields must be filled by the user in the browser window`);
perfil sempre dedicado; headers sensíveis mascarados.

## 12. Fluxos gravados

- São gravados, com seletor semântico (`role`, `name`, `nth`, contexto), apenas passos
  **bem-sucedidos** de: `open`, `click`, `type`, `select`, `press`, `scroll`, `back`, `wait_for`,
  `mock_add` e `emulate`. Ferramentas de leitura (`snapshot`, `screenshot`, `inspect`,
  `network_*`, `console_list`) e ações que falharam não são gravadas.
- Cada ação gravada termina a resposta com `step #N`.
- `flow_save` grava em `.vitral/flows/<name>.yaml`, por padrão tudo desde o último
  `flow_save` (ou desde o início da sessão); `fromStep: N` escolhe outro início.
- `params` é um mapa `{nome: "valor gravado"}`. A substituição por `{{nome}}` acontece **só** em
  campos de valor: `type.text`, `select.option`, URL do `open` e texto do `wait_for` (como
  substring). Nunca em seletores semânticos (`role`/`name`). Valores com menos de 3 caracteres são
  recusados. Parâmetro não encontrado em nenhum passo é erro. O resultado lista cada substituição
  (passo, campo, antes → depois).
- `flow_run` executa sem IA, resolvendo cada seletor no snapshot atual; em falha para no passo e
  devolve passo, motivo e snapshot.
- Autocura (`internal/heal`): cada seletor gravado leva uma assinatura (papel, nome, label,
  placeholder, `data-testid`, `id` estável, `name`, caminho do `href`, tipo do campo, região, dialog,
  título acima, cabeçalhos da tabela, posição entre iguais e na página). Se o seletor não acha
  exatamente o elemento, o `flow_run` pontua os candidatos e aceita só com nota ≥ 0,75, margem ≥ 0,15
  sobre o segundo e papel compatível (botão ↔ link; o tipo do campo nunca troca). Passo sensível
  (léxico em `internal/heal/sensitive.txt` ou o último clique) e `heal: ask` param e pedem
  confirmação (`resumeFrom` + `acceptHealing`); `heal: off` desliga. O resultado lista os passos
  curados; `flow_update` com `applyHealing=true` regrava o fluxo e guarda a versão anterior em
  `.history/`.

## 13. Fases

1. Base (stdio, config, browser, ações, diff, erros). 2. Visão (`screenshot`, `inspect`).
3. Rede e console. 4. Mock e emulação. 5. Fluxos. 6. Experiência (prompts, README, build).

## Decisões da revisão

1. Mock: a regra mais recente vence (10.2 corrigida).
2. `flow_save` com mapa de parâmetros, restrito a campos de valor, mínimo de 3 caracteres,
   relatório das substituições e erro para parâmetro não usado.
3. Passos gravados desde o último `flow_save`/início; `step #N`; `fromStep`.
4. Diff de scroll: lista o que entrou, resume o que saiu.
5. `wait_for text` usa o texto visível real do DOM.
6. Validação no Copilot é manual (`examples/vscode/CHECKLIST.md`).
7. Git local, commits por fase em Conventional Commits, sem remote.
