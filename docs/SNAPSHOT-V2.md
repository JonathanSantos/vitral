# Snapshot v2 (motor `ax`): comparação do protótipo

Protótipo atrás de `--snapshot-engine=ax` (padrão: `dom`). O motor `ax` une
`Accessibility.getFullAXTree` e `DOMSnapshot.captureSnapshot` por `backendNodeId` e gera a mesma saída
do motor `dom`. No protótipo, as refs são gravadas no DOM pelo CDP para as ações atuais funcionarem;
ações em shadow root fechado não funcionam (o `querySelector` não alcança). Só o frame principal.

Reproduzir: `go test -tags 'integration axcompare' -run TestAXCompare -v ./internal/app/` (fixtures e
tempo); `VITRAL_TEST_SNAPSHOT_ENGINE=ax make e2e` e `make integration` (suítes com o motor `ax`).

## Fixtures (snapshot normal)

| | dom | ax |
|---|---|---|
| Saída idêntica | 13 de 16 fixtures | 13 de 16 |
| Onde diferem | — | `ax` vê mais: shadow fechado (+7 nós), divs só com listener (+3 clicáveis), web components (nome do slot e botão inserido no slot) |
| Tokens (soma das 16) | 1.423 | 1.514 (+6%, todo o aumento é conteúdo que o `dom` não via) |
| Tokens do orçamento do e2e | 5.952 | 5.952 |
| Integração, e2e completo e agente heurístico | passam | passam |

## Tempo do snapshot completo (mediana de 15)

| Página | dom | ax | razão |
|---|---|---|---|
| `large.html` (500 linhas) | 25 ms | 247 ms | 9,9× |
| `big-table.html` (300 linhas) | 6,8 ms | 94 ms | 13,7× |
| `form.html` | 1,5 ms | 6,9 ms | 4,5× |

Onde o `ax` gasta (`large.html`): `getFullAXTree` ~190 ms (8.902 nós, metade de texto:
2.128 `StaticText` + 2.128 `InlineTextBox`), `captureSnapshot` ~45 ms, montagem em Go < 15 ms.
**Não cumpre o limite de 1,5×.**

## Avaliação com modelo real (sem a skill)

Cenário A: Wikipédia, 7 tarefas (busca, leitura, link, screenshot, voltar, salvar e rodar fluxo).
Cenário B: páginas difíceis, 3 tarefas (plano feito de divs com listener, web components, pagamento
dentro de shadow fechado). Tokens = respostas do Vitral + ~1.300 por screenshot (imagem).

| Cenário | Modelo | Motor | Sucesso | Chamadas | Erros | Tokens |
|---|---|---|---|---|---|---|
| A | Sonnet 5.5 | dom | 7/7 | 13 | 0 | ~12.800 |
| A | Sonnet 5.5 | ax | 7/7 | 13 | 0 | ~11.700 |
| A | Haiku 4.5 | dom | 6/7 (fluxo) | 20 | 2 | ~14.300 |
| A | Haiku 4.5 | ax | 6/7 (fluxo) | 17 | 2 | ~14.100 |
| B | Sonnet 5.5 | dom | 0,5/3 | 13 | 0 | ~3.100 |
| B | Sonnet 5.5 | ax | 3/3 (pagamento só pelo teclado) | 36 | 8 | ~2.700 |
| B | Haiku 4.5 | dom | 0,5/3 | 54 | 3 | ~8.400 |
| B | Haiku 4.5 | ax | 3/3 (pagamento só pelo teclado) | 71 | 14 | ~3.600 |

No cenário B com `ax`, os erros são todos do shadow fechado (a ref aparece, a ação não alcança: a
limitação do protótipo); os modelos contornaram com Tab e teclas. Com as ações migradas para
`backendNodeId`, esses erros sumiriam. No A, o Haiku falhou no fluxo nos dois motores (salvou passos a
mais), sem relação com o motor.

## Decisão

Pendente: o `ax` ganha em sucesso (B) e empata ou ganha em tokens, mas perde em tempo (4,5× a 14×,
limite 1,5×).
