package cli

// Mensagens para pessoas (PT-BR), num arquivo só para facilitar a tradução no futuro.
// Os textos do selo na janela do navegador ficam em internal/ux/messages.go.

const msgHelp = `Vitral: uma janela que a IA consegue ler.

O Vitral deixa o GitHub Copilot usar um navegador de verdade enquanto você acompanha.
Você não precisa deixar este comando aberto: o VS Code inicia o Vitral sozinho.

Comandos:
  vitral setup             configura o VS Code nesta pasta
  vitral setup --global    configura o VS Code para todos os projetos
  vitral doctor            confere se está tudo pronto
  vitral install-browser   baixa agora o navegador do Vitral
  vitral install-browser --from arquivo.zip
                           instala o navegador a partir de um zip baixado à mão
  vitral version           mostra a versão
`

const (
	msgVersion = "%s %s\n"

	msgInstalledFirst   = "Vitral instalado em %s (versão %s)\n"
	msgInstalledUpdate  = "Vitral atualizado em %s: versão %s (antes: %s)\n"
	msgInstalledSame    = "Vitral já está instalado em %s (versão %s)\n"
	msgInstallFailed    = "não consegui instalar o Vitral em %s: %v"
	msgMCPWritten       = "✓ %s configurado\n"
	msgMCPBackup        = "  cópia de segurança do arquivo anterior: %s\n"
	msgMCPFailed        = "não consegui atualizar %s: %v"
	msgGitWarning       = "! A pasta .vscode está no git, mas o caminho do Vitral em %s é desta máquina.\n  Para não afetar outras pessoas, prefira: vitral setup --global\n"
	msgPromptsCopied    = "✓ prompts do Vitral em %s: %d novos, %d atualizados, %d iguais, %d mantidos\n"
	msgPromptOverwrite  = "O prompt %s já existe e é diferente. Substituir? [s/N] "
	msgPromptKept       = "  mantido sem alterar: %s (rode com --yes para substituir)\n"
	msgGlobalCandidate  = "Vou configurar o %s usando:\n  servidor: %s\n  prompts:  %s\n"
	msgGlobalProfiles   = "  Atenção: você usa perfis do VS Code. Isto vale para o perfil padrão.\n"
	msgGlobalConfirm    = "Confirma? [s/N] "
	msgGlobalSkipped    = "  %s não foi alterado.\n"
	msgGlobalNoVSCode   = "não encontrei o VS Code nem o VS Code Insiders neste computador. Abra o VS Code uma vez e rode o setup de novo, ou use o setup sem --global dentro da pasta do projeto"
	msgGlobalNeedsTTY   = "o setup --global precisa de confirmação. Rode num terminal ou use --yes para confirmar"
	msgNextStepsLocal   = "\nPróximos passos:\n  1. Abra esta pasta no VS Code.\n  2. Abra .vscode/mcp.json e clique em \"Start\" acima de \"vitral\".\n  3. No chat do Copilot, escolha o modo Agent e digite / para ver os prompts do Vitral.\n"
	msgNextStepsGlobal  = "\nPróximos passos:\n  1. Reinicie o VS Code.\n  2. Rode o comando \"MCP: List Servers\", escolha \"vitral\" e clique em \"Start\".\n  3. No chat do Copilot, escolha o modo Agent e digite / para ver os prompts do Vitral.\n"
	msgUnknownCommand   = "comando desconhecido %q. Rode \"vitral\" para ver a ajuda"
	msgUnknownFlag      = "opção desconhecida %q para %s"
	msgDoctorTitle      = "Vitral %s: conferindo se está tudo pronto\n\n"
	msgDoctorOK         = "✓ %s\n"
	msgDoctorFail       = "✗ %s\n  → %s\n"
	msgDoctorAllGood    = "\nTudo pronto.\n"
	msgDoctorSomeFailed = "\nAlgo precisa de atenção (veja as setas acima).\n"

	msgCheckBrowserOK       = "navegador: %s %s (%s)"
	msgCheckBrowserPending  = "o Chrome for Testing %s do Vitral ainda não foi baixado; ele será baixado no primeiro uso (cerca de 150 MB)"
	msgCheckBrowserFallback = "; se o download falhar, o Vitral usa: %s %s"
	msgCheckBrowserMissing  = "nenhum navegador disponível"
	msgCheckDownloadOK      = "o download do navegador está acessível (%s)"
	msgCheckDownloadBlocked = "a rede bloqueou o download do navegador (%v)"
	msgFixDownload          = "use um mirror interno (VITRAL_BROWSER_MIRROR ou --browser-mirror) ou baixe o zip em outro computador e rode: vitral install-browser --from arquivo.zip"
	msgCheckDownloadSkip    = "não há Chrome for Testing para este sistema; o Vitral usa um Chromium do Puppeteer/Playwright ou o navegador do sistema"
	msgCheckLaunchSkipped   = "não testei abrir o navegador: nenhum está disponível ainda"
	msgFixLaunchMissing     = "rode: vitral install-browser"

	msgSourceExplicit = "definido em --browser-path"
	msgSourceManaged  = "Chrome for Testing do Vitral"
	msgSourceCache    = "Chromium do cache do Puppeteer/Playwright"
	msgSourceSystem   = "navegador do sistema, permitido por --allow-system-browser"

	msgNotFoundTitle       = "O Vitral não encontrou um navegador para usar.\n"
	msgNotFoundExplicit    = "  O navegador definido em --browser-path (ou VITRAL_BROWSER_PATH) não abriu: %s (%v)\n"
	msgNotFoundUnsupported = "  - Não existe Chrome for Testing para este sistema.\n"
	msgNotFoundDownload    = "  - O download do Chrome for Testing falhou: %v\n"
	msgNotFoundPending     = "  - O Chrome for Testing ainda não foi baixado.\n"
	msgNotFoundCaches      = "  - Nenhum Chromium do Puppeteer ou do Playwright funcionou neste computador.\n"
	msgNotFoundNoSystem    = "  - O Chrome/Edge do sistema não é usado sem permissão.\n"
	msgNotFoundSystemNone  = "  - Não achei Chrome nem Edge instalados.\n"
	msgNotFoundWays        = `Saídas:
  1. Mirror interno da empresa: defina VITRAL_BROWSER_MIRROR (ou --browser-mirror) com o endereço do espelho do Chrome for Testing.
  2. Zip baixado à mão: baixe %s
     em outro computador e rode: vitral install-browser --from arquivo.zip
  3. Usar o Chrome ou o Edge já instalado: adicione "--allow-system-browser" em "args" no mcp.json
     (o Vitral continua usando um perfil separado do seu).
Para conferir tudo: vitral doctor
`

	msgInstallBrowserOK          = "✓ navegador instalado: %s (versão %s)\n"
	msgInstallBrowserAlready     = "✓ o navegador do Vitral já está instalado: %s (versão %s)\n"
	msgInstallBrowserUnsupported = "não existe Chrome for Testing para este sistema. Use --browser-path, um Chromium do Puppeteer/Playwright ou --allow-system-browser"
	msgInstallBrowserFailed      = "não consegui instalar o navegador: %v"
	msgInstallBrowserNeedsFile   = "informe o arquivo: vitral install-browser --from arquivo.zip"

	msgCheckProfileOK     = "perfil do navegador pode ser gravado: %s"
	msgCheckProfileFail   = "não consigo gravar no perfil %s"
	msgFixProfile         = "confira as permissões da pasta, ou use outra com --profile-dir"
	msgCheckLaunchOK      = "o navegador abre e fecha normalmente"
	msgCheckLaunchFail    = "o navegador não abriu: %v"
	msgFixLaunch          = "feche outras janelas do Vitral e tente de novo; se continuar, reinstale o Chrome"
	msgCheckInstalledOK   = "Vitral instalado em %s (versão %s)"
	msgCheckInstalledMiss = "Vitral não está instalado em %s"
	msgCheckInstalledOld  = "a versão instalada em %s (%s) é diferente desta (%s)"
	msgFixInstall         = "rode: vitral setup"
	msgCheckMCPOK         = "%s aponta para %s"
	msgCheckMCPMissing    = "%s não tem o servidor \"vitral\""
	msgCheckMCPBadPath    = "%s aponta para %s, que não existe"
	msgCheckMCPOldVersion = "%s aponta para %s (versão %s), mas a instalada é %s"
	msgCheckMCPNone       = "nenhuma configuração do VS Code com o Vitral encontrada"
	msgFixMCP             = "rode: vitral setup (ou vitral setup --global)"

	msgDownloadStart    = "Baixando o navegador do Vitral (só na primeira vez)…\n"
	msgDownloadProgress = "Baixando o navegador do Vitral (só na primeira vez)… %s\n"
	msgDownloadUnzip    = "Preparando o navegador…\n"
	msgDownloadDone     = "Navegador pronto.\n"

	answerYes = "s"
)
