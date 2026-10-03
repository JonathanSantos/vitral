# Vitral: estratégia de testes e quality gate

> Fonte da verdade para testes e para o gate. Os números do gate vivem só em
> `.quality/thresholds.yaml`. A lógica vive no `Makefile`; o CI só chama `make`.

## 1. Objetivo e princípios

O gate garante três coisas:

1. **O código faz o que diz:** unitários, mutação, propriedades e fuzz.
2. **A IA consegue usar o Vitral:** contrato das ferramentas, orçamento de tokens, e2e com IA fake e
   avaliação com modelo real.
3. **O Vitral é seguro e não deixa lixo:** testes de segurança, vazamento de goroutines e processos,
   instalação.

Princípios:

- **Determinismo:** nada de `time.Sleep` para esperar; espere por condição, com timeout. Relógio
  injetável onde houver tempo. Viewport, timezone e locale fixos nos testes com browser.
- **Sem retry no gate.** Teste instável é bug: corrija ou coloque em quarentena (seção 10). Exceção: a
  tag `network`, fora do gate de PR.
- **Uma fonte para os limiares:** `.quality/thresholds.yaml`.
- **Lógica no Makefile, CI fino.** Trocar de GitHub Actions para outro CI é trocar só os arquivos de pipeline.
- **Estilo:** table-driven, early return, identificadores em inglês, comentários em português só para o porquê.

## 2. Organização

| Tipo | Onde | Build tag |
|---|---|---|
| Unitários, propriedades, sementes de fuzz | junto do pacote | nenhuma |
| Integração com browser | junto do pacote (`internal/app`, `cmd/vitral`) | `integration` |
| E2E via MCP (binário real por stdio) | `test/e2e/` | `e2e` |
| Agente fake heurístico | `test/fakeagent/` | nenhuma |
| Páginas de fixture | `testdata/pages/` e `testdata/pages/malicious/` | — |
| Goldens | `testdata/golden/` | — |
| Avaliação com modelo real | `test/eval/` | `eval` |
| Testes que precisam de rede | onde fizer sentido | `network` |
| Ferramenta do gate | `tools/qualitygate/` | — |

`tools/` nunca é importado pelo binário.

## 3. Unitários, race e cobertura

- `go test -race -count=1 -coverprofile` em todos os pacotes.
- **Cobertura por pacote**, com o critério de cada um no `thresholds.yaml`:
  - `unit`: só unitários. Vale para os pacotes puros (snapshot, mock, flow, ux, brand, cli, network,
    security, browser/install) e para os demais pacotes sem browser;
  - `combined`: unitários + integração (`-coverpkg=./internal/...`) + e2e (binário compilado com
    `go build -cover`, coletado via `GOCOVERDIR`). Vale para session, tools, overlay, browser, world e app;
  - `none`: sem mínimo (cola fina coberta por integração/e2e).
- O `qualitygate` mostra as duas colunas (unitária e combinada) para todo pacote.
- Pacote novo em `internal/` sem entrada no `thresholds.yaml` **falha o gate**.
- Pacote abaixo da meta entra com o valor medido como mínimo e a meta ao lado; o mínimo só sobe.

## 4. Mutação

- Ferramenta: **Gremlins** (`github.com/go-gremlins/gremlins`): mantido, funciona com o Go atual,
  roda por pacote e gera JSON.
- Escopo: os pacotes puros. No PR, só os pacotes alterados em relação à base; nota mínima
  killed / (killed + lived) de 75%.
- Exclusões só com comentário do motivo, em lista versionada.

## 5. Fuzz e propriedades

- Fuzz nativo (`FuzzXxx`), sementes em `testdata/fuzz/`, alvos: matcher do mock, parâmetros e YAML de
  fluxo, merge do `mcp.json`, JSON do Chrome for Testing, payload da binding do overlay e parse do snapshot.
  Sementes rodam em todo `go test`; crash vira semente no PR da correção.
- Propriedades com `pgregory.net/rapid` (só em testes): `diff(a, a)` vazio; aplicar o diff reconstrói
  `b`; `MarkColor` estável e cíclica; só "Retomar" tira o UX da pausa; compactação respeita `maxLines`
  e só mostra refs que existem.

## 6. Contrato e orçamento de tokens

- `testdata/golden/tools.json`: nome, descrição e schema de cada ferramenta (com e sem `--allow-eval`),
  atualizado com `-update`. Mudança de contrato aparece no diff do PR.
- Tokens estimados por `ceil(len(texto)/4)`, numa função única. Tetos por fixture e por ação dos
  cenários e2e no `thresholds.yaml`; o relatório mostra o valor e a diferença para a base versionada.
- Fixture grande obrigatória: `testdata/pages/large.html`.

## 7. E2E com IA fake

- Harness: compila o `vitral` com `-cover`, sobe por stdio com o cliente do go-sdk, Chrome for Testing
  gerenciado em headless (diretório de browsers de teste, com cache no CI), fixtures via `httptest`,
  `HOME` e perfil temporários por cenário. Em falha salva transcript, último snapshot e screenshot.
- Agente roteirizado: cenários em `test/e2e/scenarios/*.yaml`, `{{ref role name}}` resolvido da última
  resposta textual; `expect` com `contains`, `notContains`, `regex`, `isError`, `maxTokens`.
- Agente heurístico (`test/fakeagent`): objetivo numa DSL simples, enxerga só o texto das ferramentas,
  obedece só às instruções das mensagens de erro (senão falha com "error is not actionable"),
  limite de passos, registra passos e tokens. **Roda sem a skill do Vitral.**
- Cenários mínimos: formulário, dialog cobrindo, ref vencida, mock 500/timeout, `delayMs`, fluxo
  gravado e reexecutado, fluxo quebrado, pausa no meio do `flow_run`, campo de senha, navegação e
  `back`, página grande.
- Modo caótico com semente fixa no PR: o servidor nunca cai, nunca dá panic, nunca escreve fora do
  protocolo; todo erro é `isError` acionável.

## 8. Segurança

- Scripts, overlay e binding do Vitral rodam num **isolated world** do CDP (feito na etapa 0); teste de
  página maliciosa tentando descobrir globais, abrir o shadow root, envenenar protótipos, forjar
  "retomar" e esconder o selo. Esse teste não pode entrar em quarentena.
- `--allowed-hosts`, `eval_js` só com `--allow-eval`, mascaramento de headers (inclusive nos
  transcripts), senha nunca digitada, stdout só com JSON-RPC, `govulncheck` no PR.

## 9. Vazamento de recursos

`goleak` nos pacotes com concorrência; PIDs lançados expostos e conferidos ao fim dos testes com
browser; fechar o stdin no meio de uma ação mata o browser lançado (e não o do `attach`).

## 10. Demais verificações

- Lint: `golangci-lint` com configuração versionada (`.golangci.yml`), zero avisos.
- Regressão visual (noturno): goldens PNG das marcações e do selo, tolerância de 8 por canal e 0,5% de
  pixels, fontes locais, atrelados à versão do Chrome for Testing.
- Matriz Linux/macOS/Windows e instalação/atualização (noturno e release).
- Benchmarks com `benchstat` (noturno, só relatório).
- **Quarentena:** `t.Skip("quarantine: <issue> until AAAA-MM-DD")`. O `qualitygate` lista os testes em
  quarentena e falha quando o prazo vence.

## 11. Avaliação com modelo real (fora do gate)

`test/eval/` com a tag `eval`, API compatível com OpenAI por variável de ambiente, sem chave = pulada
com aviso. Cada cenário roda **com e sem a skill do Vitral**, e o relatório mostra a diferença.
Métricas: sucesso, passos, erros, tokens de entrada e saída, tempo. Nunca bloqueia merge.

## 12. O gate

| Alvo | Quando | Conteúdo | Bloqueia |
|---|---|---|---|
| `make check-fast` | pre-push local (`.githooks/pre-push`) | lint, unitários com race, cobertura unitária | sim |
| `make check` | PR (meta < 10 min) | tudo do `check-fast` + `govulncheck`, integração, e2e, contrato, tokens, segurança, vazamento, mutação nos alterados, `qualitygate` | sim |
| `make nightly` | **pendente** | `check` + mutação completa, fuzz 3 min/alvo, caos aleatório, visual, matriz, benchmarks, avaliação | — |
| `make release-check` | antes de release | `nightly` + instalação e atualização nos 3 sistemas | sim |

### Pendente: execução noturna

A intenção está registrada, mas o agendamento ainda não existe: falta decidir **onde** fica o trigger
(GitHub Actions agendado, Azure DevOps ou outro). Até lá, os alvos do noturno existem no `Makefile`
e podem ser rodados à mão; nenhum workflow `nightly.yml` foi criado.

## 13. Etapas

0. Isolated world e teste de página maliciosa (**feito**).
1. Fundação: `thresholds.yaml`, `qualitygate`, `make check`/`check-fast`, lint, `govulncheck`, `pr.yml`, pre-push.
2. E2E: harness, agentes roteirizado e heurístico, cenários, caos com semente fixa.
3. Contrato, tokens, segurança e vazamento.
4. Fuzz, propriedades e mutação.
5. Visual, matriz e instalação (alvos do Makefile).
6. Noturno: pendente (ver acima).
