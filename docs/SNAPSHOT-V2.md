# Snapshot v2: o snapshot pela árvore de acessibilidade

O snapshot vem da árvore de acessibilidade do Chrome unida ao `DOMSnapshot.captureSnapshot` por
`backendNodeId` (`internal/snapshot/ax.go`, `internal/session/axengine.go`). O motor antigo, que
percorria o DOM em JavaScript (`dom`), e a flag `--snapshot-engine` foram removidos. Só o frame
principal.

## Decisão

**Aprovado e padrão.** O `ax` empata ou ganha em sucesso e em tokens e alcança o que o `dom` não via
(shadow root fechado, divs clicáveis só por listener, conteúdo de slot). O critério de 1,5× o tempo do
`dom` era um proxy ruim e virou tetos absolutos no `thresholds.yaml`, medidos no gate. O híbrido foi
descartado.

## Refs fora do DOM

As refs não são mais gravadas na página (o atributo `data-vitral-ref` não existe mais; a página não as
vê nem as forja). O Go guarda ref ↔ `backendNodeId`, e cada ação acha o elemento com `DOM.resolveNode`
no mundo isolado, o que alcança shadow root fechado. O script do mundo isolado recebe as refs novas
de cada snapshot (`bind`, uma chamada com os elementos) para as buscas reversas (refs no texto lido,
o que cobre um alvo), e as refs que ele cria (links do texto, `wait_for` por texto) voltam para o mapa
do Go no snapshot seguinte. Contêineres de rolagem vão para o overlay por uma chave temporária.

Testes: `TestAXEngine` (ações dentro do shadow fechado: digitar, escolher opção, clicar; nenhum
atributo de ref na página; o link do texto tem a mesma ref do snapshot, e a ref criada pela leitura
vale para ações e continua no snapshot completo).

## Snapshot da área visível sem a árvore inteira

`Accessibility.getFullAXTree` custa ~185 ms no `large.html` (8.902 nós, metade só de texto) e não
aceita pedir só uma parte. Opções medidas no `large.html` (`go test -tags 'integration axperf' -run
TestAXPerf -v ./internal/app/`):

| Opção | Tempo da árvore | Observação |
|---|---|---|
| `getFullAXTree` | 185 ms | igual com `Accessibility.enable` (o custo é serializar) |
| `getChildAXNodes` com poda pelos retângulos | 90 ms | uma ida e volta por nível; exige `Accessibility.enable` |
| `getPartialAXTree` por candidato perto da viewport | 48 ms (319 chamadas) | traz só o nó e os ancestrais (não os irmãos nem os filhos, apesar do nome do parâmetro) e repete ids nos filhos dos ancestrais |
| **`queryAXTree` nas subárvores inteiras perto da viewport + `getPartialAXTree` nas folhas soltas e numa raiz por pai** | **12 ms (51 chamadas)** | escolhida |

O plano é puro (`snapshot.PlanAXFetch`): desce no DOM só onde a união das caixas da subárvore chega
perto da viewport (a mesma margem de 25% do snapshot); uma subárvore inteira dentro da margem vem
numa chamada (`queryAXTree` sem nome nem papel devolve a subárvore toda); as folhas fora dessas
subárvores vêm uma a uma; subárvores irmãs dividem a mesma chamada de ancestrais. O `full` continua
com `getFullAXTree`; o `scope` usa um `queryAXTree` só. O cache da árvore invalidado pelos eventos do
domínio Accessibility não foi necessário (e exigiria `Accessibility.enable`, que mantém a árvore viva a
cada mutação da página).

Referência no gate: `TestAXEngine/partial_ax_tree_matches_the_full_tree` compara, em todas as
fixtures (topo, meio da página e `scope`), o snapshot da busca parcial com o da árvore inteira: são
idênticos.

## Tempo final (tetos no gate)

Mediana de 15 chamadas pela ferramenta, binário de release por stdio (`TestSnapshotTiming`, e2e).

| Snapshot | Página | Protótipo | Final | Teto |
|---|---|---|---|---|
| área visível | `large.html` | ~238 ms | 57 ms | 120 ms |
| área visível | `big-table.html` | ~92 ms | 26 ms | 30 ms |
| área visível | `form.html` | ~6 ms | 11 ms¹ | 30 ms |
| completo | `large.html` | 247 ms | 244 ms | 400 ms |

¹ Inclui a ida e volta do MCP por stdio (~5 ms); a captura em si leva ~5 ms.

Fases no `large.html` (área visível): metadados e `DOMSnapshot` juntos ~44 ms (o `captureSnapshot`
serializa a página inteira, sem como recortar), árvore ~12 ms, montagem < 1 ms, validação nativa
< 1 ms.

## Regressões

Fixtures dos problemas que a comparação com o `dom` achou no `ax` (`TestAXEngine/regressions`, saída
exata e clique):

- `layout-table.html`: `<table>` sem cabeçalhos é "tabela de layout" para o Chrome; aparece como
  tabela com as linhas e sem o nome que o Chrome dá (o texto de todas as células).
- `delegated-document.html`: um listener delegado no `document` não transforma a página num
  "clickable"; os cards aparecem um a um e o clique chega ao listener.
- `body-listener.html`: um listener no `<body>` não esconde os filhos com listener próprio.

## Avaliação depois da migração

Cenário B (páginas difíceis) com o Haiku 4.5, repetido depois das ações por `backendNodeId` (números
do registro da ponte):

| Rodada | Sucesso | Chamadas | Erros | Tokens |
|---|---|---|---|---|
| protótipo (refs no DOM) | 3/3 (pagamento só pelo teclado) | 71 | 14 | ~3.600 |
| **final** | **3/3** | **11** | **0** | **~600** |

Os erros do shadow fechado sumiram: o Haiku digitou o nome, escolheu 2x e clicou em Pagar agora pelas
refs, sem contornar pelo teclado.

Cenário A (Wikipédia) com o Haiku 4.5: a falha nos dois motores era do `flow_save` (salvou passos a
mais). O `flow_save` ganhou prévia (`preview=true`: número da sessão, ação e alvo, sem salvar),
`excludeSteps`, o save logo depois da prévia com a mesma escolha, e avisos que impedem salvar um fluxo
que o `flow_run` não repetiria (alvo que depende do valor digitado, como uma sugestão de busca; busca
que termina sem Enter; `expect` que não está visível na página, com placeholder escrito à mão ou com o
valor em outra caixa), contornáveis com `ignoreWarnings=true`. Números do registro da ponte; cada
rodada acrescenta as correções do que a anterior mostrou.

| Rodada | Sem a skill | Com a skill |
|---|---|---|
| antes (motor `ax`) | 6/7 (fluxo com passos a mais), 17 chamadas, 2 erros | — |
| prévia + `excludeSteps` | 6/7 (prévia certa, save sem o intervalo) | 6/7 (salvou o clique na sugestão) |
| + save com a escolha da prévia, aviso de alvo com o valor do parâmetro | 6/7 (busca sem Enter) | 6/7 (`expect` era o título da aba) |
| + aviso de alvo com texto digitado, `expect` conferido na página | **7/7**, 22 chamadas, 1 erro | 6/7 (ignorou o aviso: falso sucesso) |
| + avisos impedem salvar, aviso de busca sem Enter | **7/7**, 18 chamadas, 3 erros (2 saves recusados) | **7/7**, 23 chamadas, 1 erro |

Na última rodada com a skill, o fluxo ficou certo (abrir e digitar com Enter), mas salvo sem `expect`
depois de duas tentativas ruins (`{{termo}}` escrito à mão e o valor em outra caixa); os dois casos
viraram avisos.

## Protótipo: comparação com o motor `dom`

Números da fase de protótipo (refs ainda gravadas no DOM, árvore inteira em todo snapshot).

### Fixtures (snapshot normal)

| | dom | ax |
|---|---|---|
| Saída idêntica | 13 de 16 fixtures | 13 de 16 |
| Onde diferem | — | `ax` vê mais: shadow fechado (+7 nós), divs só com listener (+3 clicáveis), web components (nome do slot e botão inserido no slot) |
| Tokens (soma das 16) | 1.423 | 1.514 (+6%, todo o aumento é conteúdo que o `dom` não via) |
| Tokens do orçamento do e2e | 5.952 | 5.952 |
| Integração, e2e completo e agente heurístico | passam | passam |

### Tempo do snapshot completo (mediana de 15)

| Página | dom | ax | razão |
|---|---|---|---|
| `large.html` (500 linhas) | 25 ms | 247 ms | 9,9× |
| `big-table.html` (300 linhas) | 6,8 ms | 94 ms | 13,7× |
| `form.html` | 1,5 ms | 6,9 ms | 4,5× |

Onde o `ax` gasta (`large.html`): `getFullAXTree` ~190 ms (8.902 nós, metade de texto:
2.128 `StaticText` + 2.128 `InlineTextBox`), `captureSnapshot` ~45 ms, montagem em Go < 15 ms.
O critério relativo (≤ 1,5× o `dom`) foi trocado por tetos absolutos (abaixo).

### Avaliação com modelo real (sem a skill)

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
