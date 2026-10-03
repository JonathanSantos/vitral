<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/brand/logo-dark.png">
    <img src="assets/brand/logo.png" alt="vitral" width="300">
  </picture>
</p>

<p align="center"><strong>Uma janela que a IA consegue ler.</strong></p>

O Vitral deixa o GitHub Copilot usar um navegador de verdade enquanto você acompanha: abrir telas,
preencher formulários, tirar prints, conferir o que a API respondeu e repetir tarefas gravadas. Cada
pedaço da tela ganha um número e uma cor, e você vê na janela o que a IA vai fazer antes de ela fazer.

<!-- TODO: GIF da demonstração (roteiro em docs/demo.md) -->

## Comece em 3 passos

1. **Baixe** o Vitral do seu computador na [página de versões](https://github.com/JonathanSantos/vitral/releases)
   e descompacte.
2. **Rode** dentro da pasta do seu projeto, no terminal:

   ```bash
   vitral setup
   ```

   (no Mac, se aparecer um aviso de segurança na primeira vez: `xattr -d com.apple.quarantine vitral`)
3. **Abra o Copilot** no VS Code, no modo Agent, digite `/` e escolha um prompt do Vitral.

## O que dá para pedir

| Prompt | O que faz |
|---|---|
| `/vitral-descrever-tela` | Abre uma página e explica a tela em linguagem simples. |
| `/vitral-preencher-formulario` | Preenche um formulário com os dados que você colar e para antes de enviar. |
| `/vitral-testar-api-fora` | Mostra como a tela fica quando a API dá erro, demora ou cai. |
| `/vitral-gravar-fluxo` | Faz uma tarefa com você e salva para repetir depois. |
| `/vitral-executar-fluxo` | Repete uma tarefa gravada, passo a passo. |

Ou peça do seu jeito: *"Abra o portal de pedidos e busque o pedido 1023."*

## É seguro?

- **A IA usa um navegador separado do seu**, com perfil próprio. Seus logins do dia a dia não são usados.
- **Quem digita senhas é você.** Quando aparece um campo de senha, o Vitral pausa e mostra "Sua vez".
- **Você pode pausar a qualquer momento** pelo botão "⏸ Pausar IA" no canto da janela do navegador.
- **Nada sai do seu computador** além do que a IA já vê no chat.

Algo estranho? Rode `vitral doctor`: ele confere tudo e diz o que fazer.

---

## Para desenvolvedores

Servidor MCP (stdio) em Go, binário único, que controla um Chromium via [rod](https://github.com/go-rod/rod).
Especificação em [`docs/SPEC.md`](docs/SPEC.md), identidade em [`docs/BRAND.md`](docs/BRAND.md), experiência
de uso em [`docs/UX-TASKS.md`](docs/UX-TASKS.md) e roteiro de validação manual em
[`examples/vscode/CHECKLIST.md`](examples/vscode/CHECKLIST.md).

### Ferramentas MCP

| Grupo | Ferramentas |
|---|---|
| Navegação e ações | `open`, `snapshot`, `click`, `type`, `select`, `press`, `scroll`, `wait_for`, `back` |
| Visão | `screenshot` (com marcações por ref), `inspect` |
| Rede e console | `network_list`, `network_get`, `console_list` |
| Simulação | `mock_add`, `mock_list`, `mock_clear`, `emulate` |
| Fluxos | `flow_save`, `flow_list`, `flow_run` |
| Opcional | `eval_js` (só com `--allow-eval`) |

As ações devolvem só o que mudou na tela. Enquanto a pessoa pausa, as ferramentas que agem respondem
(sem erro) que estão pausadas, e as de leitura continuam funcionando.

### Comandos e configuração

```bash
vitral                   # no terminal: ajuda; sem terminal (VS Code): servidor MCP
vitral setup [--global] [--yes]
vitral doctor
vitral version
```

O `setup` instala o binário em `~/.vitral/bin` e aponta o `.vscode/mcp.json` (ou o do usuário, com
`--global`) para ele, preservando comentários. Rodar de novo com um binário novo é a forma de atualizar.

Flags do servidor (também aceitas em `vitral.yaml` no diretório de trabalho):

| Flag | Padrão | Descrição |
|---|---|---|
| `--headless` | `false` | Navegador sem janela. |
| `--attach ws://…` / `--attach-port 9222` | | Conecta num Chrome aberto com `--remote-debugging-port` e um `--user-data-dir` separado. Nunca o fecha. |
| `--profile-dir` | `~/.vitral/profile` | Perfil dedicado do navegador. |
| `--allowed-hosts` | | Hosts permitidos para navegação e mock (globs, separados por vírgula). |
| `--allow-eval` | `false` | Registra `eval_js`. |
| `--disable-mock` | `false` | Remove as ferramentas de mock. |
| `--highlight-ms` | `350` (0 em headless) | Tempo do destaque antes de cada ação; `0` desliga. |
| `--no-overlay` | `false` | Sem destaque e sem texto no selo; o botão de pausa continua. |
| `--stability-timeout` | `10s` | Espera máxima para a página assentar depois de cada ação. |
| `--nav-timeout` | `15s` | Tempo máximo de navegação. |
| `--flows-dir` | `.vitral/flows` | Onde os fluxos são salvos. |
| `--log-level` | `info` | Logs sempre em stderr; stdout é só do protocolo MCP. |

### Arquitetura

```
cmd/vitral          entrada: comandos para pessoas ou servidor MCP
internal/cli        setup, doctor, version e mensagens em PT-BR
internal/app        monta o servidor
internal/tools      ferramentas MCP (guard: pausa, selo "trabalhando", proteção contra panic)
internal/session    ações, espera por estabilidade, diff, gravação e replay de fluxos
internal/snapshot   JS embutido que lê a página + formatação e diff em Go
internal/overlay    selo, destaque e pausa injetados na página (shadow root fechado)
internal/ux         máquina de estados do selo (pura)
internal/screenshot marcações desenhadas em Go
internal/network, console, mock, flow, browser, security, brand, config, ring
tools/brandgen      gera os PNGs da marca (não entra no binário)
```

### Limitações conhecidas

- **Pausar só com o teclado não funciona na v1.** Os botões do selo têm `tabindex="-1"` para que um
  `press Tab` da IA não caia neles e mude o foco da página. Quem usa apenas o teclado ainda não consegue
  pausar.

### Próxima versão

- Atalho de teclado para pausar e retomar, resolvendo a limitação acima sem expor os botões do selo
  à navegação por Tab da página.

### Testes e build

```bash
make test          # unitários
make integration   # integração com Chromium headless e páginas de testdata/
make lint          # go vet + staticcheck
make build         # bin/vitral
make dist          # binários para Windows, macOS e Linux em dist/
make brand         # regenera os PNGs de assets/brand
```

Releases com `goreleaser release --clean` a partir de uma tag.
