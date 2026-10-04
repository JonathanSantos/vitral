# Vitral: cursor visual da IA

Complementa o `UX-TASKS.md` (destaque, selo e pausa) e usa as cores e o tom do `docs/BRAND.md`.
Regras do projeto: Go puro e testável onde der, overlay no isolated world, testes no padrão do
`docs/TESTING.md`. Números que parecerem ruins na prática são ajustados e justificados com o
`cursorlab` (seção 10). As decisões tomadas na aprovação do plano estão na seção 12.

## 1. Objetivo e princípios

Com o navegador visível, quem olha vê **um cursor da IA se movendo como uma pessoa moveria**: sai de
onde estava, faz um arco leve, acelera, desacelera, às vezes passa um pouco do ponto e corrige, para e
clica. A pessoa acompanha e entende o que a IA está fazendo.

1. **É só visual.** O cursor é desenhado no overlay. Os eventos reais continuam simples (2.3); o
   caminho humano não é reproduzido em eventos reais.
2. **Não é para enganar detecção de bots.** Nada de falsificar `navigator.webdriver`, randomizar eventos
   reais ou qualquer técnica anti-detecção. O Vitral continua identificável (faixa do Chrome for
   Testing e selo).
3. **Honesto:** o cursor nunca se move à toa. Todo movimento corresponde a uma ação real.
4. **Rápido o bastante:** orçamento de tempo por ação (seção 5); com `instant`, praticamente zero.
5. **Nunca atrapalha:** `pointer-events: none`; não aparece no snapshot nem no screenshot da IA; a
   ação não depende da animação terminar (há timeout).
6. **Só existe com janela visível.** Em headless ou com `--no-overlay`, desligado e sem atraso
   (exceto `--cursor=force`, seção 7).

## 2. Arquitetura

### 2.1 Responsabilidades

- **Go planeja** (`internal/cursor`, pacote puro): recebe posição atual, alvo, perfil e um `*rand.Rand`
  com semente e devolve um plano: keyframes `(t ms, x, y)`, forma ao chegar, instante do clique.
  Toda a física fica aqui, com unitários, propriedades e mutação.
- **O overlay toca** (`cursor.js` dentro do `<vitral-overlay>`): interpola os keyframes com
  `requestAnimationFrame`, desenha e avisa pela binding (`cursor_done` com o id do plano).
- **Go espera e age:** aguarda o `cursor_done` (timeout = duração do plano + 300 ms) e dispara o
  evento real. Em timeout, segue com a ação e loga em debug.

### 2.2 Estado

Posição atual do cursor em Go, por página. Após navegação ou reload, o overlay é reidratado com o
cursor na última posição (limitada à viewport).

### 2.3 Eventos reais

- `click`: um `mouseMoved` direto para o **mesmo ponto final** do cursor visual, depois `mousePressed`
  e `mouseReleased` nesse ponto, nos tempos de 4.6. O que a pessoa vê é onde o clique acontece.
- Nenhum `mouseMoved` intermediário: nada no caminho recebe hover.
- Se o alvo precisa de rolagem, ela acontece antes do plano (5.1).

## 3. Visual do cursor

| Forma | Quando |
|---|---|
| `arrow` | padrão |
| `pointer` | alvo com `cursor` computado `pointer` |
| `text` | campo de texto ou `cursor: text` |

A forma muda nos últimos 80 ms do movimento. Cerca de 20×24 CSS px, Chumbo `#1E1B2E` com contorno de
1,5 px em Vidro `#F7F5F0` e `drop-shadow` 0 2px 4px a 25%.

- **Etiqueta:** pílula no canto inferior direito com o `icon-small` e "IA"; visível em movimento e
  ação, some 1,5 s depois de parado. Diferencia o cursor da IA do cursor real da pessoa.
- **Clique:** o cursor encolhe para 90% no press; um anel na cor da marcação da ref (`MarkColor`) vai
  de 0 a 28 px de raio em 300 ms, opacidade 0,6 → 0.
- **Primeira aparição:** na primeira ação da sessão, o cursor surge do selo (fade-in de 150 ms).

## 4. Mecânica do movimento

Valores de partida da velocidade `normal`, todos no `Profile` (`internal/cursor/profile.go`).

### 4.1 Ponto de destino

- Normal em torno do centro, σ = 15% da largura e da altura, limitada aos 70% internos.
- Campo de texto: terço esquerdo e meio vertical.
- Validação na página e alternativas: seção 12.

### 4.2 Duração (Fitts)

`ID = log2(D/W + 1)` (W = menor dimensão, mínimo 8 px), `T = 100 + 110·ID` ms, ±8% (normal limitada),
entre 250 e 900 ms em `normal`. `D < 3 px`: sem movimento, só o clique.

### 4.3 Trajetória

Bézier cúbica com controles a 30% e 70% da reta, deslocados na perpendicular: `D × U(0,04; 0,12)` no
primeiro e metade no segundo, do mesmo lado. Lado sorteado; repete o anterior com 65% de chance.
Desvio máximo da reta ≤ 15% de D e ≤ 120 px.

### 4.4 Velocidade

Jerk mínimo `s(τ) = 10τ³ − 15τ⁴ + 6τ⁵` sobre comprimento de arco, com deformação de tempo para o pico
ficar entre 35% e 45% da duração.

### 4.5 Submovimentos

Com `ID ≥ 3,5`: 70% param antes (92–97% de D, erro lateral até 0,3 W), 30% passam do ponto (2–6% de D,
até 24 px); pausa de 20–60 ms e corretivo de 80–160 ms quase reto (curvatura ≤ 2%), mesmo perfil.
Alvo que se moveu mais de 4 px: recalcula o ponto (mesma regra de 12.2) e faz um corretivo.

### 4.6 Tremor e tempos de clique

Ruído suavizado de 0,4 px a 8–12 Hz na perpendicular, com envelope zero nas duas pontas (seção 12).
Mira de 70–140 ms; press → release de 60–110 ms.

### 4.7 Keyframes

A cada 16 ms, interpolação linear no overlay, CSS px da viewport, serialização compacta (array plano).

### 4.8 Determinismo

Todo sorteio usa o `*rand.Rand` injetado. Semente por sessão, aleatória por padrão e fixa via
`--cursor-seed`. Mesma semente e mesmas ações geram os mesmos planos.

## 5. Coreografia por ação

Tempo visual por ação em `normal` ≤ 1,3 s (movimento + mira + clique), sem contar rolagem.

- **5.1 `click`:** rolagem suave até 600 ms se preciso (cursor parado); recalcula o retângulo e o
  ponto; o destaque UX-1 começa junto com o movimento; movimento, mira, press/release com o anel; o
  evento real sai no press visual. `--highlight-ms` vira o tempo mínimo de destaque visível antes do
  clique (a mira se estende se preciso).
- **5.2 `type`:** como o `click`, com a forma `text`; depois o cursor se afasta 40–80 px para baixo e à
  direita em 150–250 ms. Ritmo de digitação só com janela visível: 25–45 ms por caractere, teto de
  1,5 s, o resto via `insertText`. Com `submit: true`, keycap "Enter".
- **5.3 `select`:** vai e clica; o valor é aplicado como hoje; etiqueta "Selecionado: {opção}" por 800 ms.
- **5.4 `scroll`:** com `ref`, vai até o container; sem `ref`, ao centro da viewport. Passos de roda de
  ~100 px a cada 60–90 ms, teto de 800 ms, o resto de uma vez. Cursor parado durante a rolagem.
- **5.5 `press`:** sem movimento; keycap com o nome da tecla acima do selo por 600 ms.
- **5.6 `open` e `back`:** cursor a 40%; selo mostra "Abrindo {host}" ou "Voltando"; depois reaparece.
- **5.7 Leitura** (`snapshot`, `screenshot`, `inspect`, `network_*`, `console_list`): não mexe o cursor.
- **5.8 `flow_run`:** coreografia de cada passo, velocidade padrão `fast` (configurável).

## 6. Comportamentos de estado

- **6.1 Parado:** fica onde está; etiqueta some em 1,5 s; 70% de opacidade após 5 s.
- **6.2 Pausa e "Sua vez":** fade-out em 200 ms ao pausar, fade-in ao retomar; em "Sua vez" para ao
  lado do campo e some, o destaque continua.
- **6.3 Cursor real:** coexistem; a etiqueta "IA" diferencia; o cursor do sistema não é escondido.
- **6.4 Falha:** vai até o alvo, faz "não" (3 oscilações de 6 px em 240 ms) e mostra "Coberto por um
  diálogo" ou "Botão desabilitado" por 1,2 s. O erro para a IA não muda.
- **6.5 Aba oculta:** plano aplicado na hora e `cursor_done` imediato (ver também seção 12).
- **6.6 Movimento reduzido:** sem trajeto, fade no destino em 120 ms, sem tremor nem oscilação, anel
  estático de 200 ms.
- **6.7 Zoom e resize:** coordenadas recalculadas na hora do plano; posição limitada à nova viewport.

## 7. Configuração

| Flag / config | Valores | Padrão |
|---|---|---|
| `--cursor` | `on` / `off` / `force` | `on` com janela visível; `off` em headless |
| `--cursor-speed` | `slow` (×1,6), `normal`, `fast` (×0,6), `instant` | `normal` (fluxos: `fast`) |
| `--cursor-seed` | inteiro | aleatória por sessão |
| `--type-pacing` | `on` / `off` | `on` com janela visível |

`force` liga o cursor mesmo sem janela visível (testes e gravação de demos). Em `instant`: sem trajeto,
fade de 80 ms no destino e anel; mira e press no mínimo. O multiplicador vale para movimento, mira,
submovimentos e tetos. `--no-overlay` desliga o cursor. `slow` é para demos e o GIF do README.

## 8. Requisitos de implementação

- `internal/cursor` só com funções puras (`Candidates`, `Compute`, `FreeArea`), sem rod nem CDP.
- A integração obtém o retângulo, valida candidatos, chama `Compute`, envia ao overlay, espera
  `cursor_done` e dispara o evento real.
- O overlay não calcula física. Cores de `internal/brand`, textos para pessoas em
  `internal/ux/messages.go`. Mesmo `<vitral-overlay>` do selo: isolated world, fora do snapshot e do
  screenshot, sem rede.

## 9. Testes

- **Unitários e propriedades** (`rapid`): extremos, tempo (Fitts), curvatura, sem laços, pico de
  velocidade, submovimentos e overshoot, tremor zero nas pontas, determinismo, robustez (D = 0, alvo de
  1 px, coordenadas negativas ou enormes, NaN), keyframes a 16 ms com o último no destino.
- **Fuzz** `FuzzPlan`; **mutação** com `internal/cursor` na lista de pacotes puros (85% de cobertura).
- **Integração** (headless com `--cursor=force`): clique no ponto final do plano; nenhum `mousemove`
  intermediário; cursor fora do snapshot e do screenshot; aba oculta antes e durante a animação;
  `prefers-reduced-motion` sem keyframes intermediários; falha por cobertura com oscilação e erro
  inalterado; clique nunca no filho interativo (12.2); orçamento ≤ 1,3 s + margem.
- **Visual** (noturno): cursor parado com etiqueta, mão sobre botão, anel no meio, keycap.
- **E2E:** continua em headless sem cursor e sem ficar mais lento.

## 10. `tools/cursorlab`

`make cursorlab` (ou `go run ./tools/cursorlab -speed slow -seed 2 -set FittsB=120`) gera
`.quality/out/cursorlab/index.html`: 50 trajetórias por cenário (perto/grande, longe/pequeno, diagonal,
vertical, campo de texto), velocidade × tempo com o pico marcado, estatísticas e um palco que toca os
planos com o mesmo `cursor.js` do overlay, inclusive uma sequência que sai de onde parou.
`testdata/pages/cursor-playground.html` e `make demo-cursor` vêm na etapa 4.

## 11. Entrega

1. `internal/cursor` com testes, propriedades e fuzz, mais o `cursorlab`; parar para validação.
2. Overlay: desenho, formas, etiqueta, anel, primeira aparição, parado, pausa, falha, aba oculta,
   movimento reduzido.
3. Integração com `click`, `type`, `select`, `scroll`, `press`, `open`/`back`, `flow_run` e UX-1.
4. Testes de integração e visuais, `cursor-playground` e `make demo-cursor`.
5. README ("É seguro?": o cursor é só visual) e `docs/demo.md` com o roteiro do GIF em `slow`.

## 12. Decisões da aprovação

1. **Candidatos numa ida só.** O Go sorteia sempre 5 candidatos mais o ponto de repouso (centro; em
   campo de texto, o centro do terço esquerdo) com `Candidates`, e a página valida a lista num único
   eval. Como todos os sorteios acontecem sempre, a mesma semente gera o mesmo plano qualquer que seja o
   candidato aceito.
2. **Validação do ponto de clique.** Um candidato só vale se `elementFromPoint` devolver o próprio alvo
   ou um descendente **não interativo** (interativos: `a[href]`, `button`, `input`, `select`,
   `textarea`, `[role]` interativo, `[onclick]`, `[tabindex>=0]`, `[contenteditable]`). Ex.: num card
   clicável com uma lixeira dentro, um ponto sobre a lixeira não vale como clique no card. Se nenhum
   candidato passar, tenta o ponto de repouso com a mesma regra; se falhar, usa o centro do maior
   retângulo do alvo livre de descendentes interativos (`FreeArea`, calculado em Go com os retângulos
   que a página devolve no mesmo eval). Só então vale a falha "covered by". A mesma regra vale para o
   recálculo do alvo que se moveu (4.5).
3. **Teto de 250–900 ms** vale para o movimento inteiro (principal + pausa + corretivo); com mira e
   press, o máximo fica em ~1,15 s. Se `--highlight-ms` for maior que movimento + mira, ele estende a
   ação além de 1,3 s; o teste de orçamento usa o padrão.
4. **`--cursor=force`**: flag documentada, "liga o cursor mesmo sem janela visível (testes e gravação de
   demos)".
5. **Movimento reduzido e aba oculta** são consultados (`matchMedia`, `visibilityState`) antes de
   planejar: o plano já vai sem keyframes intermediários. Além disso, o overlay ouve `visibilitychange`
   durante a animação: se a aba ficar oculta no meio, pula para o keyframe final e envia `cursor_done`
   na hora (com teste de integração).
6. **Tremor** com envelope que zera nas duas pontas (10% do começo e do fim) e aplicado na
   perpendicular da corda do movimento: assim ele nunca faz a projeção na direção principal recuar, e a
   propriedade "sem laços" tolera só o overshoot e o arredondamento de 0,1 px.
7. **Pico de velocidade** medido no movimento principal; a deformação de tempo é `τ^k`, com `k` achado
   pela forma fechada do pico (`τ^k = (3k−1)/(5k−1)`), sorteando o alvo em 36–44% para a amostragem em
   keyframes não sair de 35–45%.
8. **"Abrindo {host}" e "Voltando"** entram na máquina de estados pura do `ux` como rótulo temporário.
9. **`cursor_done` no press.** O overlay avisa a chegada no instante do press (ou no fim do movimento,
   quando não há clique) e termina press, anel e release sozinho. O Go dispara `mousePressed` ao
   receber, espera o tempo de press do plano e dispara `mouseReleased`: o evento real sai no press
   visual (5.1) e a espera continua sendo "duração do plano + 300 ms" (2.1), contada até o press.
10. **Um só ícone.** O `cursor.js` define o ícone pequeno (`VITRAL_ICON`) usado pelo selo e pela
    etiqueta "IA"; o overlay injeta o `cursor.js` no mesmo escopo, dentro do shadow root fechado.

11. **`select` clica só no visual.** O cursor vai até o select e faz mira, press e anel, mas sem evento
    real: com janela visível, um clique real abriria a lista nativa do sistema, que ficaria aberta por
    cima da página. O valor continua sendo aplicado como antes.
12. **`scroll` com `ref`.** A ferramenta rola *até* o elemento: a rolagem acontece com o cursor parado
    e depois o cursor vai até o elemento, sem clicar. Sem `ref`, o cursor vai ao centro da viewport e
    rola em passos de roda reais no ponto do cursor.
13. **Mira e press num segundo plano.** O movimento é tocado sem clique; depois o Go confere se o alvo
    andou mais de 4 px (4.5) e toca a mira e o press a partir de onde o cursor parou, com correção
    se precisar. Assim a correção sempre acontece antes do press visual.
14. **"Abrindo {host}" e "Voltando"** só aparecem com o cursor ativo: sem janela visível ninguém vê o
    selo, e o comportamento em headless fica igual ao de antes.
15. **Sem ponto livre e sem cobertura conhecida** (todo o alvo coberto por filhos interativos): a IA
    recebe "element X has no free spot to click…", com a instrução de clicar no filho.

### Ajustes da etapa 1 (achados pelos testes)

- O tremor é limitado a 2% da distância (num movimento de 3 px, 0,4 px de tremor seria maior que o
  próprio arco e quebraria o limite de curvatura).
- O arco e o corretivo usam só o que sobra do limite de desvio depois do erro lateral e do tremor (o
  fuzz achou um corretivo de movimento enorme passando de 120 px).
- Os candidatos são arredondados para 0,1 px **para dentro** da área permitida (num alvo minúsculo o
  arredondamento comum empurrava o ponto para fora).
- Em Go, tipo e função não podem ter o mesmo nome: o tipo é `Plan` e a função é `Compute`.
