# Checklist de validação manual no VS Code (Copilot)

Pré-requisitos: binário `vitral` no PATH (ou caminho absoluto em `command`) e o
`mcp.json` desta pasta copiado para `.vscode/mcp.json` do projeto de teste.

## Fase 1: base

1. Abra o projeto no VS Code e rode **MCP: List Servers**. O servidor `vitral` deve aparecer.
   Clique em **Start**. O status vira "Running" sem abrir nenhuma janela de browser ainda.
2. Abra o chat do Copilot no modo **Agent** e confira em **Tools** (ícone de ferramentas) que aparecem
   `open`, `snapshot`, `click`, `type`, `select`, `press`, `scroll`, `wait_for` e `back`, e que **não**
   aparece `eval_js`.
3. Peça: *"Abra https://example.com e me diga o que tem na tela."*
   - Uma janela do Chrome abre (visível) e navega.
   - A resposta da ferramenta `open` mostra `page: "Example Domain"` e um `link` com ref `[eN]`.
4. Peça: *"Clique no link."* A resposta da ferramenta mostra só o diff (URL nova ou snapshot novo).
5. Peça: *"Volte para a página anterior."* (`back`).
6. Num site com formulário de login, peça para digitar a senha. A ferramenta `type` deve recusar com
   `password fields must be filled by the user in the browser window`.
7. Feche a aba do Chrome manualmente e peça um `snapshot`. Uma aba nova abre, sem erro.
8. Em **MCP: List Servers → browser → Show Output**, confira que não há erros de protocolo
   (logs do servidor aparecem como stderr).
9. Pare o servidor (**Stop**). A janela do Chrome lançada por ele fecha.

## Fase 2: visão

1. Com uma página aberta, peça: *"Tire um print da tela."*
   - O chat mostra a imagem com caixas rosa (elementos clicáveis) e azuis (textos/títulos), cada uma com
     a etiqueta `eN`.
   - Peça um `snapshot` e confira que as refs do print batem com as do texto.
2. Peça: *"Qual é a cor e o fundo do botão principal?"*. A IA usa `inspect` e responde com valores
   hex (ex.: `color: #ffffff`, `background: #0055aa`).
3. Peça: *"Tire um print só do botão X"* (`screenshot` com `ref`). Vem só o recorte do elemento.
4. Peça: *"Tire um print da página inteira"* (`fullPage`). A imagem inclui o rodapé.

## Fase 3: rede e console

1. Abra uma tela que chama uma API (ex.: a listagem do seu sistema) e peça:
   *"Quais chamadas de API a tela fez?"*. A IA usa `network_list` e mostra linhas
   `id METHOD STATUS URL (ms, tamanho)`.
2. Peça: *"Mostre o retorno da chamada X"*. Em `network_get` vem o JSON formatado, e headers como
   `Authorization` e `Cookie` aparecem como `***`.
3. Peça: *"Tem algum erro no console?"*. Em `console_list` vêm as mensagens com `arquivo:linha`.
4. Clique em algo que gera erro. A resposta da própria ação já traz `console: N new error(s)` e/ou
   `network: N failed request(s)`.

## Fase 4: mock e emulação

1. Numa tela que chama uma API, peça: *"Simule a API de pedidos retornando erro 500 e me mostre como a
   tela fica."* A IA usa `mock_add` (ex.: `*/api/orders*`, status 500), repete a ação e a resposta mostra
   a mensagem de erro da tela, mais `network: … 500 … mocked`.
2. Peça: *"Agora deixe a API lenta (3 segundos) e mostre o carregamento."* (`delayMs`). A resposta da
   ação mostra o indicador de loading e, em seguida, `wait_for` mostra o resultado final.
3. Peça: *"Liste os mocks ativos"* (`mock_list`) e depois *"remova todos"* (`mock_clear`).
4. Peça: *"Mostre a tela como num celular"* (`emulate` com `device: mobile`) e tire um print.
   Depois peça para voltar ao desktop.
5. Peça: *"Simule ficar sem internet"* (`network: offline`) e clique em algo que chama a API.
   Volte com `network: none`.
6. Rode o servidor com `--disable-mock` em `args` e confira que as ferramentas `mock_*` somem.

## Fase 5: fluxos

1. Faça um fluxo pelo chat (ex.: abrir a tela, buscar o pedido `1023`, filtrar). Cada ação bem-sucedida
   termina com `step #N`.
2. Peça: *"Salve isso como fluxo `buscar-pedido`, com o número do pedido como parâmetro `numero`."*
   A IA chama `flow_save` com `params: {numero: "1023"}`. A resposta lista os passos e cada substituição
   (`step 2 type.text: "1023" → "{{numero}}"`).
3. Confira o arquivo `.vitral/flows/buscar-pedido.yaml` no projeto.
4. Peça: *"Liste os fluxos"* (`flow_list`) e *"rode o fluxo buscar-pedido com o número 2048"*
   (`flow_run`). Os passos rodam sozinhos e a resposta mostra `ok` em cada um.
5. Edite o YAML trocando o nome de um botão para algo que não existe e rode de novo. O relatório para
   no passo certo com `FAILED`, o motivo e a lista de botões que existem na tela.

## Fase 6: experiência

1. Gere o binário (`make build` ou `make dist`) e rode `vitral --version`.
2. Copie `examples/vscode/.github/prompts` para `.github/prompts` do projeto de teste.
3. No chat, digite `/`: devem aparecer `descrever-tela`, `testar-api-fora` e `executar-fluxo`.
4. Rode `/descrever-tela` com uma URL. O VS Code pede o endereço e a IA abre e descreve a tela.
5. Rode `/testar-api-fora` numa tela com API. A IA passa pelos cenários de erro 500, sem conexão, lentidão
   e lista vazia, e no fim remove os mocks.
6. Rode `/executar-fluxo` com um fluxo salvo na Fase 5.
7. Siga o README como se fosse uma pessoa não técnica (baixar, colar o `mcp.json`, Start, usar).

## Fase 8: selo, destaque e pausa

1. Abra uma página pelo chat. No canto inferior direito da janela aparece o selo
   "Vitral está usando esta janela" com o botão "⏸ Pausar IA".
2. Peça uma ação (ex.: clicar num botão). O elemento pisca com um contorno colorido e a etiqueta `eN`
   antes do clique, e o selo mostra "A IA está trabalhando…".
3. Clique em "⏸ Pausar IA". O selo mostra "Pausado. Você está no controle.". Peça outra ação:
   a IA responde que você pausou e pede para clicar em "Retomar". Peça um print: funciona, e o selo
   não aparece na imagem.
4. Clique em "▶ Retomar" e peça a ação de novo: funciona e considera o que você mudou na tela.
5. Numa tela de login, peça para a IA preencher a senha. O campo fica destacado e o selo mostra
   "Sua vez: preencha a senha e clique em Retomar".
6. Recolha o selo pelo "–"; ele vira um botão redondo com o ícone, e continua recolhido ao navegar.
7. Rode um fluxo salvo e veja "Executando fluxo: nome · passo i de n". Pause no meio: o fluxo para
   antes do próximo passo e a IA avisa onde parou.
8. Rode com `"--no-overlay"` em `args`: sem destaque e sem texto de status, mas o botão de pausa continua.
