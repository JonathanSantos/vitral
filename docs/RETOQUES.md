# Vitral: Retoques

Especificação para aprovação. Complementa o `UX-TASKS.md` (selo e pausa), segue as cores e o tom do
`docs/BRAND.md` e o padrão de testes do `docs/TESTING.md`. Medições que decidem um método (como a do
`session_save`) vêm antes do código (seção 9).

## 1. Objetivo e princípios

Hoje o Vitral serve para a IA **usar** sites. Com os Retoques, ela também **retoca** sites: muda o visual,
acrescenta uma conveniência ou cria uma extensão. A pessoa passa a usar esse navegador retocado no dia
a dia com `vitral open`, com ou sem a IA.

1. **Só neste navegador.** Um retoque muda o que esta pessoa vê, neste navegador. O site, o servidor e as
   outras pessoas não são tocados (a mesma linguagem dos mocks).
2. **Sempre visível e reversível.** Dá para saber o que está retocado e ver o original a um clique.
3. **Prévia antes de fixar.** Nada fica valendo sem a pessoa ver o antes e o depois e dizer "sim".
4. **Retoque muda como o site se parece e acrescenta conveniências; tarefas são fluxos.** Um retoque
   nunca envia, salva, paga ou exclui nada sozinho. Isso continua sendo `flow_run`, com as regras de
   sempre.
5. **Sem LLM dentro do Vitral.** A IA escreve o retoque; o Vitral aplica, mostra, guarda e confere.
6. **Fluido.** `vitral open` abre o navegador retocado sem avisos nem perguntas (decisão 12.1).

## 2. Modos de uso

### 2.1 Experimentar ao vivo

> **Pessoa:** deixa esse sistema com tema escuro e a fonte da tabela maior
>
> **IA:** Vou aplicar só nesta aba, para você ver. *(`retouch_preview`)* Pronto: fundo escuro, texto claro
> e a tabela com fonte de 16 px. Antes e depois: *[dois prints]*. Quer manter sempre neste site?

- Vale só na aba atual, sem recarregar a página, e some ao recarregar.
- O selo mostra **"Prévia de retoque · Ver original"**. "Ver original" alterna na hora entre antes e
  depois.
- A resposta da ferramenta diz o que pegou ("tema: 1.240 elementos; tabela: 1 de 1") para a IA não
  dizer que funcionou quando um seletor não pegou nada.

### 2.2 Fixar

> **Pessoa:** sim, mantém
>
> **IA:** Salvei o retoque **"Tema escuro"** em `sistema.empresa.com`. Ele vale sempre que você abrir esse
> site no Vitral. Para desligar, use o botão do Vitral na barra do navegador.

- Igual aos fluxos: `retouch_save` com `preview=true` lista o que será salvo (nome, sites, o que muda em
  palavras simples, se tem JavaScript). Depois, o save só com o nome salva exatamente essa prévia.
- **Escopo explícito:** esta página (padrão de URL), este site (padrão) ou todos os sites.
- A partir daí, o retoque vale nas sessões da IA e no `vitral open`.

### 2.3 Usar como navegador: `vitral open`

```bash
vitral open                      # abre o navegador do Vitral com os retoques, no perfil default
vitral open sistema.empresa.com  # já abre nesse endereço
vitral open --profile hml        # outro perfil (logins e retoques dele)
vitral open --sem-retoques       # saída de emergência: tudo desligado nesta janela
```

- É o **mesmo perfil** que a IA usa (decisão 12.2): os logins feitos num valem no outro.
- Sem aviso e sem pergunta (decisão 12.1). É um navegador comum para a pessoa: sem a faixa de
  automação, com isolamento de sites e bloqueio de pop-ups ligados (seção 6.2).
- O botão **Vitral** na barra de extensões abre um painel com os retoques do site atual: liga e desliga
  cada um, "Ver original" e "Desligar todos neste site".

### 2.4 Criar extensões

> **Pessoa:** cria uma extensão que mostra há quantos dias cada pedido está atrasado
>
> **IA:** Vou criar a extensão em `extensoes/atraso-pedidos/` e carregar no navegador para testar.
> *(escreve os arquivos, `extension_load`, abre a lista de pedidos, lê a tela)* A coluna "Atraso" apareceu
> nas 12 linhas; o pedido 10234 está com 5 dias. O console da extensão está sem erros.

- A IA escreve os arquivos com as ferramentas do próprio Copilot, na pasta do projeto. O Vitral
  **carrega, recarrega, mostra o console e testa** com as ferramentas de sempre (snapshot, click,
  console_list).
- Extensões carregadas assim também valem no `vitral open` até a pessoa remover.
- `vitral extension export <nome>` gera o zip para instalar em outro Chrome (modo desenvolvedor, à mão).
  O Vitral nunca instala nada no Chrome da pessoa.

### 2.5 Compartilhar

- Retoques são pessoais por padrão (`~/.vitral/retoques/`, seção 5).
- `vitral retoque export <nome> --to tema-escuro.zip` e `vitral retoque import` levam um retoque para
  outra pessoa. O import mostra o que o retoque faz e, se tiver JavaScript, pede confirmação no terminal.

## 3. UX em detalhe

### 3.1 Onde a pessoa vê o que está retocado

| Situação | Onde aparece |
|---|---|
| Sessão da IA | Selo: "2 retoques neste site", com o botão "Ver original" |
| Prévia | Selo: "Prévia de retoque · Ver original · Descartar" |
| `vitral open` | Botão Vitral na barra de extensões, com o número de retoques ativos no site; painel com interruptores |
| Retoque quebrado | Ícone com um ponto âmbar e "Tema escuro não encontrou nada nesta página" no painel (3.4) |

"Ver original" desliga todos os retoques da aba enquanto estiver ligado, sem recarregar. É o mesmo
comportamento no selo e no painel.

### 3.2 Antes e depois

`retouch_preview` devolve o resumo do que pegou (barato: a IA itera várias vezes). Os prints de antes e
depois (sem selo nem cursor, como no `screenshot`, ~1.300 tokens cada) vêm no `retouch_save` com
`preview=true`, que é o momento de mostrar à pessoa, ou no `retouch_preview` com `shots=true`. A skill
orienta a mostrar os dois antes de perguntar se a pessoa quer manter.

### 3.3 JavaScript

CSS é o padrão e cobre a maior parte dos pedidos (tema, fonte, esconder, destacar, reordenar). Quando o
pedido precisa de JavaScript (uma coluna calculada, um botão "copiar", um atalho de teclado):

- A prévia diz, em palavras simples, o que o código faz e o que ele lê da página: "lê a coluna Data e
  acrescenta uma coluna Atraso".
- O save de retoque com JavaScript pede um "sim" que cita isso. Uma confirmação dada para o CSS não vale
  para o JavaScript.
- O Vitral confere o código antes de salvar (análise simples, sem LLM) e devolve **avisos** para a IA
  repassar à pessoa:
  - manda dados para outro endereço (`fetch`, `XMLHttpRequest`, `sendBeacon`, `WebSocket`, imagem ou
    formulário apontando para outro host);
  - clica, envia formulário ou chama `submit()` (isso é tarefa de fluxo, princípio 4);
  - lê campos de senha.
  Um save com aviso é recusado; a pessoa confirma com `ignoreWarnings=true`, como nos fluxos.
- Por padrão o código roda no **mundo isolado** da extensão (lê e muda o DOM, mas não vê as variáveis
  da página). `world: page` só quando o retoque precisa de uma função da página, e isso aparece na prévia.

### 3.3.1 CSS também é conferido

CSS não é risco zero: um seletor de atributo (`input[value^="a"]`) com `url()` apontando para outro
host manda para fora o que a pessoa digita, letra por letra. O Vitral confere o CSS antes de salvar e
avisa (save recusado sem `ignoreWarnings=true`, como no JavaScript) quando há referência a outro host em
`url()`, `@import`, `image-set()` ou `src` de `@font-face`. Referências ao próprio site e `data:` não
avisam.

### 3.4 Quando o site muda

Cada regra do retoque guarda o que deveria encontrar ("a tabela de pedidos", seletor e contagem
esperada). No `vitral open`, se uma regra não pega nada numa página onde costumava pegar, o painel
mostra o aviso. Na sessão da IA, a resposta do `open` e do `snapshot` ganha uma linha
`retouches: "Tema escuro" matched nothing on this page`, e a IA oferece consertar (nova prévia → "sim").
É a mesma ideia da autocura dos fluxos, sem conserto automático.

### 3.5 Quando algo quebra

- O painel tem "Desligar todos neste site".
- `vitral open --sem-retoques` abre com tudo desligado.
- `vitral retoque off <nome>` e `vitral retoque list` no terminal.
- Um retoque que lança erro no console aparece no painel com o erro, e a IA vê com `console_list`.

## 4. Ferramentas MCP

| Ferramenta | Faz |
|---|---|
| `retouch_preview` | `css`, `js` (opcional), `world`. Aplica na aba atual, devolve o que pegou e os prints de antes e depois |
| `retouch_discard` | Tira a prévia da aba |
| `retouch_save` | `name`, `description`, `scope` (`page`, `site` ou `all`), `preview=true`. Só com o nome, salva a última prévia |
| `retouch_list` | Retoques do site atual e de todos, com estado (ligado, desligado, quebrado) |
| `retouch_toggle` | Liga ou desliga um retoque (pede "sim" para ligar um com JavaScript importado) |
| `extension_load` | `path`. Carrega (ou recarrega) uma extensão desempacotada da pasta do projeto |
| `extension_list` | Extensões carregadas, com erros do manifest |
| `extension_logs` | Console do service worker e dos content scripts da extensão |

Desligadas com `--no-retouches`. Excluir um retoque ou remover uma extensão fica só no terminal ou no
painel, nunca com a IA.

## 5. Modelo de dados

```
~/.vitral/retoques/
  tema-escuro/
    retoque.yaml      # nome, descrição, escopo (padrões de URL), regras, mundo, criado/alterado
    style.css
    script.js         # opcional
    .history/         # versões anteriores, como nos fluxos
  .state.json         # ligado/desligado por retoque e por perfil
~/.vitral/extensions.json   # extensões desempacotadas carregadas (caminhos), por perfil
```

- Retoques guardam **código**, nunca dados da página (o mesmo princípio do mapa de telas).
- Permissões 0600/0700, por usuário. Retoques por projeto (`.vitral/retoques/` na pasta) ficam para
  depois, se a equipe pedir.

## 6. Arquitetura

### 6.1 Tudo vira extensão

Os retoques fixados são compilados numa extensão gerada, **Vitral Retoques** (MV3, em
`~/.vitral/build/retoques/`), carregada com `--load-extension` em todo lançamento:

- um content script estático por retoque (`matches` do escopo, `run_at: document_start` para o CSS
  entrar antes de pintar, `world` conforme o retoque);
- o CSS é aplicado como `<style>` controlado pelo content script (dá para desligar sem recarregar);
- `chrome.storage` guarda o estado (ligado, desligado, "ver original" por aba);
- o action popup é o painel da seção 3.1.

Assim o retoque funciona igual com ou sem a IA conectada, e exportar é copiar a pasta. Extensões da
pessoa (2.4) entram na mesma lista do `--load-extension`.

**Prévia** (2.1) não passa pela extensão: o Vitral injeta direto na aba pelo CDP, no mundo isolado do
Vitral, e tira no descarte. **Fixar** grava o retoque, regenera a extensão e mantém a injeção na sessão
atual até o próximo lançamento, ou recarrega a extensão sem reiniciar se a medição 9.2 permitir.

### 6.2 Dois jeitos de lançar o mesmo perfil

| | Sessão da IA (hoje) | `vitral open` |
|---|---|---|
| Bandeiras de automação (`enable-automation`, sem isolamento de sites, sem bloqueio de pop-ups…) | sim | **não** |
| Extensões (`--load-extension`) | Retoques + as da pessoa | Retoques + as da pessoa |
| Porta ou pipe de depuração | sim | **não** (decisão 14.1: os sites veem um Chrome comum) |
| Cofre de senhas | o mesmo nos dois, sempre (decisão 14.2) | o mesmo nos dois, sempre |

### 6.3 Mesmo perfil, um navegador só

*Substituída pela decisão 14.1: o `vitral open` não tem depuração, então a IA não entra nele. O texto
abaixo fica como registro do que foi medido e descartado.*

O Chrome não abre o mesmo perfil duas vezes. Então:

- **A IA começa e o `vitral open` já está aberto:** o Vitral acha o navegador pelo `DevToolsActivePort` do
  perfil e se conecta a ele (como o `--attach` de hoje), numa aba nova. A pessoa vê a IA trabalhando
  na mesma janela que já estava usando. Para manter o Vitral identificável (princípio do projeto), a
  sessão liga `Emulation.setAutomationOverride` (o `navigator.webdriver` volta a ser `true`) nas abas da
  IA, além do selo.
- **`vitral open` com a sessão da IA aberta:** abre uma janela nova no navegador que já está rodando.
- Ao fechar: a IA nunca fecha um navegador que a pessoa abriu com `vitral open`.

### 6.4 Navegador atualizado

Com o público "qualquer pessoa na web" (decisão 12.3), o navegador do `vitral open` precisa das
correções de segurança. Proposta: o `vitral open` usa o **Chrome for Testing Stable mais recente**
(conferido no máximo uma vez por dia, baixado em segundo plano, trocado no próximo lançamento), e a
versão fixada (`install.PinnedVersion`) continua valendo para os testes e para quem pedir
`--browser-path`. A sessão da IA usa a mesma versão que o `vitral open`, para o perfil não ir e voltar
entre versões.

## 7. Segurança e privacidade

- A IA nunca vê o conteúdo de outro retoque além do nome, da descrição e do que ele muda; o código só
  quando ela vai editá-lo.
- Retoques não ganham acesso a sessões salvas, senhas ou ao estado do Vitral.
- JavaScript: avisos da seção 3.3, mundo isolado por padrão e "sim" próprio.
- Pedidos que contornam controle de acesso (paywall, bloqueio de conteúdo pago, limites de uso) ou
  verificação humana ficam fora: a skill orienta a recusar, como hoje com CAPTCHA.
- `--allowed-hosts` também limita onde um retoque pode ser salvo (escopo dentro dos hosts permitidos).

## 8. Skill

Trechos novos da skill do Vitral:

- Quando a pessoa pedir para mudar a aparência ou o funcionamento de um site, use `retouch_preview`
  primeiro, mostre o antes e o depois e pergunte se ela quer manter.
- Prefira CSS. Use JavaScript só quando o pedido precisar, e explique em palavras simples o que ele faz.
- Retoques não fazem tarefas (enviar, salvar, excluir). Para isso, sugira um fluxo.
- Depois de salvar, diga onde ligar e desligar (o botão Vitral na barra do navegador) e como abrir o
  navegador retocado (`vitral open`).

## 9. Medições antes do código

1. **`--load-extension` no Chrome for Testing fixado e no Stable mais recente** (o Chrome comum deixou de
   aceitar a bandeira; o Chrome for Testing deve continuar aceitando): carregar, content script em
   `document_start`, action popup, `world: MAIN`.
2. **Recarregar a extensão sem reiniciar:** `Extensions.loadUnpacked` pelo CDP (exige
   `--enable-unsafe-extension-debugging` e conexão por pipe), ou `chrome.runtime.reload()` pelo service
   worker da própria extensão via `Target`. O que vencer decide se "fixar" vale na hora.
3. **Chave do perfil no macOS:** abrir o mesmo perfil com e sem `use-mock-keychain` e conferir se
   cookies e senhas sobrevivem. Se não sobreviverem, as duas formas de lançar usam a mesma bandeira.
4. **Attach ao navegador do `vitral open`:** `DevToolsActivePort`, aba nova, `setAutomationOverride`, e o
   que acontece com o selo e o cursor numa janela que a pessoa também está usando.
5. **Custo:** tempo de lançamento com 0, 10 e 50 retoques; tempo até o CSS aparecer (sem "piscar" o
   original).

## 10. Testes

- **Unitários:** compilação do retoque para o manifest (escopos, mundos, `document_start`), análise do
  JavaScript (cada aviso da seção 3.3, com casos que não devem avisar), estado ligado e desligado,
  `.history`.
- **Integração** (Chrome for Testing com `--load-extension`): prévia e descarte sem recarregar; "Ver
  original"; retoque fixado aplicado num documento novo antes da primeira pintura; retoque quebrado
  detectado; `extension_load` e `extension_logs` com uma extensão de exemplo em `testdata/extensions/`.
- **E2E:** cenário de tema escuro do pedido ao `vitral open` (lançado sem bandeiras de automação,
  retoque ativo, painel com o interruptor).
- **Visual:** selo com prévia, painel da extensão.
- **Com modelo** (como `docs/AVALIACAO-SONNET.md`): tema escuro, coluna calculada com JavaScript, uma
  extensão do zero e um pedido de contornar paywall (deve recusar).

## 11. Entrega

1. Medições da seção 9; parar para validação. **Feito** (seções 13 e 14).
2. Retoques de CSS, em fatias, cada uma com `make check` e commit:
   1. pacote puro `internal/retouch`: modelo, armazenamento com `.history`, escopo → padrões de URL,
      compilação para a extensão MV3, conferência do CSS (3.3.1);
   2. sessão e ferramentas (`retouch_preview`, `retouch_discard`, `retouch_save`, `retouch_list`,
      `retouch_toggle`), extensão carregada no lançamento e recarregada no save, `--no-retouches`;
   3. o que roda na página: content script, painel da extensão, "Ver original" no painel e no selo;
   4. `vitral open`, com o perfil ocupado nos dois sentidos (14.1);
   5. cofre de senhas do sistema e a migração no macOS (14.2), por último e sozinho: até lá os dois
      modos usam a mesma chave de hoje;
   6. skill, README e avaliação com modelo.
   A atualização diária do navegador (6.4) fica para uma fase própria, depois desta.
   **Fatias 1 a 6 feitas** (commits "Retoques, fatia N"); a avaliação com modelo está em
   `docs/AVALIACAO-SONNET.md`.
3. Retoques com JavaScript: avisos, "sim" próprio, mundos.
4. Extensões da pessoa: `extension_load`, `extension_list`, `extension_logs`, export.
5. Retoques quebrados (3.4), export e import, avaliação com modelo e README.

## 12. Decisões

1. **Sem aviso no `vitral open`**, nem para sites de fora: quem usa o comando sabe o que está fazendo, e
   a experiência tem que ser fluida.
2. **Mesmo perfil da IA**: os logins valem nos dois.
3. **Público:** qualquer pessoa na web (por isso a seção 6.4).
4. **Nome:** Retoques.

## 13. Resultados das medições (4 de outubro de 2026)

`go test -tags retouchdiag -run TestRetouchDiag -v ./internal/browser/` (as partes com janela:
`VITRAL_DIAG_VISIBLE=1`, janela fora da tela). Chrome for Testing 154.0.8037.92, que hoje é também o
Stable mais recente, então a medição vale para a seção 6.4.

| # | Pergunta | Resultado |
|---|---|---|
| 9.1 | `--load-extension` funciona? | **Sim**, com e sem as bandeiras de automação. O `<style>` posto em `document_start` já vale no primeiro script da página (sem piscar o original); `world: MAIN` funciona; o service worker sobe ~40 ms depois do navegador. |
| 9.2 | Recarregar sem reiniciar? | **Sim, pelo `Extensions.loadUnpacked`**, na conexão de sempre, sem bandeira especial: carrega uma extensão nova e, chamado de novo no mesmo caminho, recarrega a extensão em ~33 ms, valendo na navegação seguinte. `chrome.runtime.reload()` pelo service worker não serve (o worker novo não subiu em 10 s). Então "fixar" vale na hora e `extension_load` não reinicia nada. |
| 9.3 | Chave do perfil no macOS | **Não medido**: criaria um item no Keychain da pessoa e pode abrir uma janela do sistema. Decisão 14.2. |
| 9.4 | Attach ao navegador do `vitral open` | `DevToolsActivePort` existe e o attach funciona, `setAutomationOverride` funciona. **Mas a porta de depuração sozinha já faz `navigator.webdriver = true` em todas as abas**, mesmo sem `enable-automation`; o pipe (`--remote-debugging-pipe`) também. Sem nenhuma das duas, `false`. E um segundo lançamento no mesmo perfil falha (`SingletonLock`), com ou sem janela: não há entrega para o navegador que já está aberto. |
| 9.5 | Custo | Lançamento até o service worker: ~260–410 ms com 0, 10 ou 50 retoques (sem diferença mensurável). O CSS já vale no primeiro script da página nos três casos. |

O que muda na especificação: a seção 6.3 supunha que o `vitral open` poderia ficar com a porta de
depuração aberta para a IA entrar depois. Com a 9.4, isso faria o navegador da pessoa se apresentar
como automatizado para todos os sites (mais CAPTCHAs e bloqueios). Decisão 14.1.

## 14. Decisões das medições

1. **Navegador normal no `vitral open` (14.1 a).** Sem porta nem pipe de depuração: os sites o veem como
   um Chrome comum (`navigator.webdriver = false`). Logins e retoques são do perfil, no disco, e valem
   nos dois modos, mas não ao mesmo tempo:
   - a IA começa com o `vitral open` aberto no mesmo perfil: o Vitral responde com um erro acionável
     ("the person is using this profile in vitral open; ask them, in one sentence, to close that window,
     then call again"), e a skill orienta a IA a pedir isso em uma frase;
   - `vitral open` com a IA trabalhando no perfil: abre uma janela nova no navegador da IA (pela porta
     que ele já tem) e termina.
   A seção 6.3 (attach ao navegador do `vitral open`) fica descartada.
2. **Cofre de senhas do sistema, igual nos dois modos (14.2, para Linux, macOS e Windows).**

   | Sistema | Onde o Chrome guarda a chave dos cookies e senhas | O que o Vitral faz |
   |---|---|---|
   | macOS | Keychain | Perfis persistentes **sem** `use-mock-keychain`; o macOS pergunta uma vez se o Chrome for Testing pode usar o Keychain |
   | Windows | DPAPI (ligada ao usuário do Windows) | Nada: já é assim, sem pergunta |
   | Linux | libsecret/KWallet, ou "basic" sem chaveiro | Nada: o Chrome detecta sozinho, e lê os dois formatos |

   - Controlado por uma configuração explícita (`--password-store system|mock`, padrão `system`), não
     por "perfil persistente": os testes, o e2e, as pontes de avaliação e o `demo-cursor` usam
     `mock` (eles também usam pastas de perfil próprias). Perfis temporários do `--session` usam `mock`.
   - A primeira pergunta do macOS acontece onde a pessoa está olhando (`vitral setup` ou `vitral open`),
     nunca no primeiro `open` da IA (que poderia ficar parado esperando o clique até estourar o tempo).
   - **Migração no macOS**, uma vez por perfil (marca no próprio perfil): antes do primeiro lançamento
     sem `use-mock-keychain`, o Vitral abre o perfil como antes (sem janela), lê os cookies pelo CDP
     (só em memória), fecha, abre no modo novo e regrava os cookies; a marca de "migrado" só é gravada
     depois que a regravação deu certo. Senhas salvas no gerenciador do Chrome não migram
     (a pessoa digita as senhas; o Vitral não as salva).
   - Assim a medição 9.3 deixa de ser necessária: os dois modos usam sempre o mesmo cofre.
