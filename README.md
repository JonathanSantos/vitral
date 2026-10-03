# browser-mcp

Deixa o **GitHub Copilot** (no VS Code) usar um navegador de verdade por você: abrir telas, clicar,
preencher formulários, tirar prints, olhar as chamadas de API e repetir tarefas gravadas.

Você continua vendo tudo: o navegador abre numa janela normal na sua tela.

## 1. Instalar

1. Baixe o arquivo do seu computador na página de releases:
   - Windows: `browser-mcp_..._windows_amd64.zip`
   - Mac com chip Apple (M1, M2…): `browser-mcp_..._darwin_arm64.tar.gz`
   - Mac com Intel: `browser-mcp_..._darwin_amd64.tar.gz`
   - Linux: `browser-mcp_..._linux_amd64.tar.gz`
2. Descompacte e guarde o programa `browser-mcp` (no Windows, `browser-mcp.exe`) numa pasta fixa, por
   exemplo `C:\Ferramentas\` ou `~/ferramentas/`.
3. **Só no Mac:** na primeira vez, abra o Terminal e rode o comando abaixo (troque o caminho se
   tiver guardado em outro lugar). Ele libera o programa baixado da internet:

   ```bash
   xattr -d com.apple.quarantine ~/ferramentas/browser-mcp
   ```

Se você tiver o Google Chrome instalado, ele é usado. Se não tiver, o programa baixa o Chromium
sozinho na primeira vez (pode levar um ou dois minutos).

## 2. Ligar no VS Code

1. No seu projeto, crie o arquivo `.vscode/mcp.json` com o conteúdo abaixo.
2. Troque `browser-mcp` em `command` pelo caminho completo do programa, por exemplo
   `C:\\Ferramentas\\browser-mcp.exe` ou `/Users/voce/ferramentas/browser-mcp`.

```json
{
  "servers": {
    "browser": {
      "type": "stdio",
      "command": "browser-mcp",
      "args": ["--profile-dir", "${userHome}/.browser-mcp/profile"]
    }
  }
}
```

3. Salve o arquivo. Aparece um botão **Start** em cima do servidor `browser`: clique nele.
4. Abra o chat do Copilot no modo **Agent**. Pronto.

## 3. Usar

Peça em português mesmo, por exemplo:

- *"Abra https://portal.empresa.com/pedidos e me diga o que tem na tela."*
- *"Busque o pedido 1023 e me mostre os detalhes."*
- *"Tire um print da tela."*
- *"A tela mostrou erro. O que a API respondeu?"*

### Prompts prontos

Copie a pasta [`examples/vscode/.github/prompts`](examples/vscode/.github/prompts) para dentro do seu
projeto (fica `.github/prompts`). No chat, digite `/` e escolha:

| Prompt | Para quê |
|---|---|
| `/descrever-tela` | Abre uma página e explica o que tem nela. |
| `/testar-api-fora` | Mostra como a tela fica com a API com erro, lenta ou fora do ar (sem mexer no servidor real). |
| `/executar-fluxo` | Repete uma tarefa gravada, passo a passo. |

### Gravar e repetir tarefas

Depois de fazer uma tarefa pelo chat, peça: *"Salve isso como fluxo `buscar-pedido`, com o número do
pedido como parâmetro."* Da próxima vez: *"Rode o fluxo buscar-pedido com o número 2048."* O fluxo roda
sozinho, sem a IA improvisar. Os fluxos ficam em `.browser-mcp/flows/` no seu projeto.

## Segurança

- **Login é sempre com você.** A IA não digita senhas. Quando aparecer uma tela de login, entre pela
  janela do navegador. O navegador usa um perfil próprio (`~/.browser-mcp/profile`), separado do seu
  Chrome do dia a dia, e lembra o login nas próximas vezes.
- Valores de `Authorization`, `Cookie` e afins aparecem como `***` para a IA.
- Para limitar os sites, adicione em `args`: `"--allowed-hosts", "portal.empresa.com,*.empresa.com"`.

## Problemas comuns

| Problema | O que fazer |
|---|---|
| O servidor não inicia | Confira o caminho em `command`. No VS Code: **MCP: List Servers → browser → Show Output**. |
| "profile may be in use" | Outra janela do VS Code já está usando o navegador. Feche-a, ou use outro `--profile-dir`. |
| A IA diz "ref not found" | A tela mudou. Ela mesma deve pedir um `snapshot` novo; se não pedir, peça você. |
| Quero o navegador escondido | Adicione `"--headless"` em `args`. |

---

## Para desenvolvedores

Requer Go (versão estável mais recente). A spec completa está em [`docs/SPEC.md`](docs/SPEC.md) e o
roteiro de validação manual no Copilot em [`examples/vscode/CHECKLIST.md`](examples/vscode/CHECKLIST.md).

```bash
make build        # bin/browser-mcp
make dist         # binários de todas as plataformas em dist/
make check        # vet + staticcheck + testes unitários + integração (Chromium headless)
```

Os releases saem com `goreleaser release --clean` a partir de uma tag.

Principais flags (também aceitas em `browser-mcp.yaml` no diretório de trabalho):

| Flag | Padrão | Descrição |
|---|---|---|
| `--headless` | `false` | Navegador sem janela. |
| `--attach ws://…` / `--attach-port 9222` | | Conecta num Chrome já aberto com `--remote-debugging-port` (o Chrome exige um `--user-data-dir` separado). Nunca fecha esse Chrome. |
| `--profile-dir` | `~/.browser-mcp/profile` | Perfil dedicado do navegador. |
| `--allowed-hosts` | | Hosts permitidos para navegação e mock (globs, separados por vírgula). |
| `--allow-eval` | `false` | Registra a ferramenta `eval_js`. |
| `--disable-mock` | `false` | Remove as ferramentas de mock. |
| `--stability-timeout` | `10s` | Espera máxima para a página assentar depois de cada ação. |
| `--nav-timeout` | `15s` | Tempo máximo de navegação. |
| `--flows-dir` | `.browser-mcp/flows` | Onde os fluxos são salvos. |
| `--log-level` | `info` | Logs vão sempre para stderr; stdout é só do protocolo MCP. |

Ferramentas: `open`, `snapshot`, `click`, `type`, `select`, `press`, `scroll`, `wait_for`, `back`,
`screenshot`, `inspect`, `network_list`, `network_get`, `console_list`, `mock_add`, `mock_list`,
`mock_clear`, `emulate`, `flow_save`, `flow_list`, `flow_run` e `eval_js` (opcional).
