# Vitral: nome, ícone e identidade

> Fonte da verdade para nome, ícone, cores, tipografia e tom de voz. As tarefas de experiência de uso
> estão em `docs/UX-TASKS.md`. O quadro visual original do ícone (ícone e assinatura, variações,
> marcações e paleta) foi entregue como `Vitral — ícone.html`; a variação escolhida é a **A · Janela em arco**.

---

## 1. O nome

**Vitral.** Um browser é uma janela, e um vitral é uma janela feita de pedaços coloridos. É exatamente o que o projeto faz: quebra a página em pedaços numerados (as refs) e mostra para a IA um print com cada pedaço marcado numa cor.

- Escrita: **Vitral** em textos e títulos; **`vitral`** (minúsculo) no binário, em comandos, caminhos e no logotipo.
- Taglines aprovadas:
  - Principal: **"Uma janela que a IA consegue ler."**
  - Secundária: **"A IA enxerga a tela, pedaço por pedaço."**

## 2. Renome do projeto (feito)

| Antes | Depois |
|---|---|
| binário e `cmd/browser-mcp` | `vitral` e `cmd/vitral` |
| constante do nome do servidor MCP | `vitral` |
| `browser-mcp.yaml` | `vitral.yaml` |
| `~/.browser-mcp/profile` | `~/.vitral/profile` |
| `.browser-mcp/flows/` | `.vitral/flows/` |
| `data-mcp-ref` | `data-vitral-ref` |
| `window.__mcpSnapshot` | `window.__vitralSnapshot` |
| chave em `mcp.json` (`"browser"`) | `"vitral"` |

Module path: `github.com/JonathanSantos/vitral`. Commit: `refactor: rename project to vitral`.

## 3. Ícone

### 3.1 Conceito

Uma janela com o topo em arco, dividida em quatro vidros coloridos por um contorno escuro grosso (o "chumbo" de um vitral). O vidro azul tem um pequeno selo com o número **1**, que é a mesma marcação que a IA desenha em cada pedaço da tela.

- Em tamanhos pequenos (**≤ 48 px**), o selo some e o ícone vira quatro quadradinhos coloridos numa moldura. Isso é proposital e mantém o favicon legível.
- No fundo escuro, o contorno inverte: o chumbo vira vidro claro.

### 3.2 Arquivos

Em `assets/brand/` (`viewBox` 512×512, sem fontes externas além do "1"):

**`icon.svg`**: ícone principal, para fundos claros.

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" role="img" aria-label="Vitral">
  <defs>
    <clipPath id="vitral-glass"><path d="M124 436 V208 A132 132 0 0 1 388 208 V436 Z"/></clipPath>
  </defs>
  <path d="M96 464 V208 A160 160 0 0 1 416 208 V464 Z" fill="#1E1B2E"/>
  <g clip-path="url(#vitral-glass)">
    <rect x="124" y="40" width="120" height="180" fill="#3E63DD"/>
    <rect x="268" y="40" width="120" height="180" fill="#F5A524"/>
    <rect x="124" y="244" width="120" height="192" fill="#30A46C"/>
    <rect x="268" y="244" width="120" height="192" fill="#E5484D"/>
  </g>
  <rect x="148" y="160" width="48" height="48" rx="10" fill="#F7F5F0"/>
  <text x="172" y="196" text-anchor="middle" font-family="Space Grotesk, Inter, Arial, sans-serif" font-weight="700" font-size="34" fill="#1E1B2E">1</text>
</svg>
```

**`icon-dark.svg`**: para fundos escuros. Igual ao `icon.svg`, com três trocas:
- a moldura (primeiro `path`) passa a `fill="#F7F5F0"`;
- o selo (`rect` com `rx="10"`) passa a `fill="#1E1B2E"`;
- o texto "1" passa a `fill="#F7F5F0"`.

**`icon-small.svg`**: para 16, 32 e 48 px. Igual ao `icon.svg`, **sem** o selo e sem o texto.

**`icon.png` e tamanhos**: PNGs de 16, 32, 48, 128, 256 e 512 px. `icon-small.svg` até 48 px e `icon.svg` acima disso. Gerados por um programa em `tools/` (sem adicionar dependência ao binário).

### 3.3 Regras de uso

- Não distorça, não gire, não aplique gradiente nem sombra no ícone.
- Mantenha uma área livre ao redor de pelo menos 1/8 da largura do ícone.
- Não reordene as cores dos vidros: safira, âmbar, esmeralda e rubi, em sentido horário a partir do topo esquerdo.
- Como ícone de app (tile), use fundo branco com cantos de 23% do lado, ou fundo Chumbo com `icon-dark.svg`.

## 4. Logotipo

- O logotipo é o **ícone + "vitral"** em Space Grotesk Bold, minúsculo, com tracking −0,04em.
- A altura do ícone é cerca de 0,9× a altura da palavra, e o espaço entre os dois fica em torno de 0,2× a altura do ícone.
- **README:** o GitHub não carrega fontes dentro de SVG em `<img>`. No topo do README, use o `icon.svg` mais um título em Markdown, ou um `assets/brand/logo.png` renderizado (claro e escuro) com `<picture>` e `prefers-color-scheme`. Nunca use um SVG do logotipo que dependa de fonte.

## 5. Cores

As cores da marca são as mesmas que a IA usa para marcar a tela. Assim, o print anotado já carrega a identidade do projeto.

| Token | Nome | Hex | Uso |
|---|---|---|---|
| `ink` | Chumbo | `#1E1B2E` | moldura do ícone, texto principal, fundos escuros |
| `primary` | Safira | `#3E63DD` | cor primária, marcação 1 |
| `mark2` | Rubi | `#E5484D` | marcação 2 |
| `mark3` | Esmeralda | `#30A46C` | marcação 3 |
| `mark4` | Âmbar | `#F5A524` | marcação 4 |
| `mark5` | Ametista | `#8E4EC6` | marcação 5 |
| `paper` | Vidro | `#F7F5F0` | fundo claro, selo do ícone |
| `muted` | Texto secundário | `#5B5770` | legendas (contraste ≥ 4,5:1 sobre Vidro) |

### 5.1 Marcações no screenshot

- **Ordem das cores:** a ref `eN` recebe `marks[(N-1) % 5]`, com `marks = [Safira, Rubi, Esmeralda, Âmbar, Ametista]`. A cor de uma ref é estável entre prints.
- **Contorno:** 3 px na cor da marcação, a 6 px de distância da bounding box e com raio de 6 px.
- **Etiqueta:** retângulo cheio na cor da marcação, no canto superior esquerdo, sobrepondo o contorno, com o texto da ref (`e12`).
  - Texto **branco** sobre Safira e Ametista.
  - Texto **Chumbo** sobre Rubi, Esmeralda e Âmbar, por contraste.
- **Etiquetas sobrepostas:** se duas etiquetas se cruzarem, desloque a segunda para baixo do contorno.

### 5.2 No código

- `internal/brand` tem as cores como constantes (`color.RGBA`) e a função pura `MarkColor(ref int) (fill, text color.RGBA)`, com teste table-driven.
- O pacote `screenshot` e qualquer CSS injetado na página leem dessa fonte. O CSS recebe as cores como custom properties geradas a partir das constantes.
- Nenhum hex duplicado em outro lugar.

## 6. Tipografia

| Uso | Fonte | Observação |
|---|---|---|
| Logotipo e títulos de marca | Space Grotesk (500, 700) | só onde a fonte puder ser carregada |
| Texto de interface (selo, botão de pausa) | Inter (400, 500, 600) | com fallback `system-ui, sans-serif` |
| Snapshots, refs e exemplos de código | JetBrains Mono (400, 500) | fallback `ui-monospace, monospace` |

- **Overlays na página controlada:** o selo e o botão de pausa **não** carregam fontes da rede. Use `system-ui, sans-serif` e `ui-monospace, monospace`.
- **Screenshots em Go:** `basicfont`.

## 7. Tom de voz

- **Para pessoas** (README, selo na janela, mensagens da CLI): português do Brasil, caloroso, curto, sem jargão. "a IA", "o navegador", "a tela", "fluxo".
  - Bom: "Vitral está usando esta janela."
  - Ruim: "CDP session attached to target."
- **Para a IA** (descrições e erros das ferramentas): inglês, imperativo e acionável. Não traduzir.
- **Logs em stderr:** inglês, estruturados com `slog`.
- **Proibido:** emoji em textos da interface, exceto ⏸ e ▶ no botão de pausa.

## 8. README

Duas camadas: primeiro quem não é técnico; o resto numa seção técnica separada.

1. **Topo:** ícone, "Vitral" e a tagline principal.
2. Um parágrafo dizendo o que é, com uma imagem ou GIF da janela com as marcações (placeholder comentado enquanto não existir).
3. **"Comece em 3 passos":** baixar, rodar `vitral setup` e abrir o Copilot e escolher um prompt.
4. **"O que dá para pedir":** os prompts prontos, com uma linha cada.
5. **"É seguro?":** a IA usa um navegador separado do seu; quem digita senhas é você; você pode pausar a qualquer momento; nada sai do seu computador além do que a IA já vê.
6. **"Para desenvolvedores":** ferramentas MCP, flags, configuração, arquitetura, testes e build.

A parte para pessoas deve caber em uma tela.
