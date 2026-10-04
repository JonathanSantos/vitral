# Checklist de validação manual no VS Code (Copilot)

Pré-requisitos: `vitral setup` rodado na pasta do projeto de teste (ou o `mcp.json` desta pasta
copiado para `.vscode/mcp.json`, com o caminho do binário em `command`).

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

## Fase 6 e 9: experiência (CLI, prompts e README)

1. Gere o binário (`make build` ou `make dist`). No terminal, rode `vitral` sem argumentos: aparece a
   ajuda em português e o comando termina (não fica esperando).
2. Numa pasta de teste, rode `vitral setup`:
   - o binário é copiado para `~/.vitral/bin/vitral` e a saída mostra a versão;
   - `.vscode/mcp.json` ganha a entrada `vitral` apontando para esse caminho (se já existia, as outras
     entradas e os comentários continuam e há um `.bak` ao lado);
   - `.github/prompts/` recebe os 5 prompts `vitral-*`;
   - se a pasta `.vscode` estiver no git, aparece o aviso sugerindo `--global`.
3. Edite um dos prompts e rode o setup de novo: ele pergunta antes de substituir.
4. Rode `vitral setup --global`: ele mostra o caminho do `mcp.json` do usuário (VS Code e/ou Insiders) e
   pede confirmação antes de gravar.
5. Rode `vitral doctor`: todos os itens com ✓. Apague `~/.vitral/bin/vitral` e rode de novo: o item da
   instalação aparece com ✗ e a ação sugerida.
6. No VS Code, digite `/` no chat e confira os prompts `vitral-descrever-tela`, `vitral-preencher-formulario`,
   `vitral-testar-api-fora`, `vitral-gravar-fluxo` e `vitral-executar-fluxo`. Rode cada um uma vez:
   - o `preencher-formulario` para antes de enviar e pede "sim";
   - o `testar-api-fora` termina sem mocks ativos (`mock_list` vazio).
7. Siga o README como uma pessoa não técnica: a parte de cima deve caber em uma tela.

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

## Cursor visual da IA

1. Peça um clique: um cursor com a etiqueta "IA" surge do selo, vai até o botão com um arco leve,
   para, e o anel colorido marca o clique. Mexa o seu mouse ao mesmo tempo: os dois convivem.
2. Peça para preencher um campo de texto: o cursor vira "I", clica, se afasta e o texto aparece
   tecla a tecla.
3. Peça para escolher uma opção num select: o cursor clica, a lista nativa **não** abre, e aparece
   "Selecionado: …" ao lado do cursor.
4. Num card clicável com um botão de excluir dentro, peça para abrir o card: o clique nunca cai no
   botão de excluir.
5. Pause: o cursor some. Retome: ele volta no mesmo lugar. Navegue: ele reaparece onde estava.
6. Peça um clique num botão desabilitado: o cursor faz "não" com a cabeça e mostra "Botão
   desabilitado"; a IA recebe o mesmo erro de sempre.
7. Com `"--cursor-speed", "slow"` em `args`, o movimento fica mais lento; com `"--cursor", "off"`, some.

## Navegador do Vitral (Chrome for Testing)

1. Com `~/.vitral/browsers` vazio, rode `vitral doctor`: ele diz que o Chrome for Testing ainda não foi
   baixado, mostra o plano B (se houver) e confirma se o download está acessível.
2. Peça qualquer ação no Copilot: o painel de saída do servidor mostra "Baixando o navegador do Vitral
   (só na primeira vez)… NN%" e depois abre um Chrome que **não** é o seu (ícone "Chrome for Testing").
3. Rode `vitral doctor` de novo: "navegador: Chrome for Testing do Vitral <versão> (<caminho>)".
4. Rede bloqueada: defina `VITRAL_BROWSER_MIRROR` para um endereço inválido e rode `vitral doctor`:
   o item do download aparece com ✗ e a sugestão de mirror ou zip manual.
5. Zip manual: baixe a URL que o doctor mostra, apague `~/.vitral/browsers` e rode
   `vitral install-browser --from <arquivo.zip>`.
6. Sem navegador nenhum (mirror inválido, sem caches, sem `--allow-system-browser`): uma ação no Copilot
   falha e o painel de saída explica as três saídas em português.
7. Com `"--allow-system-browser"` em `args` e sem Chrome for Testing nem caches, o Vitral abre o seu
   Chrome, mas com o perfil separado (sem seus logins).
