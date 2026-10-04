package cli

// Mensagens para pessoas (PT-BR), num arquivo só para facilitar a tradução no futuro.
// Os textos do selo na janela do navegador ficam em internal/ux/messages.go.

const msgHelp = `Vitral: uma janela que a IA consegue ler.

O Vitral deixa o GitHub Copilot usar um navegador de verdade enquanto você acompanha.
Você não precisa deixar este comando aberto: o VS Code inicia o Vitral sozinho.

Comandos:
  vitral setup             configura o Vitral nesta pasta (.mcp.json)
  vitral setup --global    configura o Vitral para todos os projetos (~/.copilot/mcp-config.json)
  vitral status            mostra onde o Vitral está configurado e o que ele vai usar
  vitral disable           desliga o Vitral nos arquivos de configuração (--workspace ou --global)
  vitral enable            liga de novo
  vitral session save <nome> [--profile p]
                           salva o login do perfil (cookies e localStorage) para rodar sem janela
  vitral session list      lista as sessões salvas (sem mostrar o conteúdo)
  vitral session delete <nome>
  vitral map list          lista os mapas de telas (um por site)
  vitral map show <site>   mostra o mapa de um site (telas e caminhos, nunca dados)
  vitral map export <site> --to arquivo.json | import arquivo.json | clear <site>
  vitral doctor            confere se está tudo pronto
  vitral install-browser   baixa agora o navegador do Vitral
  vitral install-browser --from arquivo.zip
                           instala o navegador a partir de um zip baixado à mão
  vitral version           mostra a versão
`

const (
	msgVersion = "%s %s\n"

	msgInstalledFirst  = "Vitral instalado em %s (versão %s)\n"
	msgInstalledUpdate = "Vitral atualizado em %s: versão %s (antes: %s)\n"
	msgInstalledSame   = "Vitral já está instalado em %s (versão %s)\n"
	msgInstallFailed   = "não consegui instalar o Vitral em %s: %v"
	msgMCPWritten      = "✓ %s configurado\n"
	msgMCPBackup       = "  cópia de segurança do arquivo anterior: %s\n"
	msgMCPFailed       = "não consegui atualizar %s: %v"
	msgGitWarning      = "! %s está no git, mas o caminho do Vitral nele é desta máquina.\n  Para não afetar outras pessoas, prefira: vitral setup --global\n"
	msgPromptsCopied   = "✓ prompts do Vitral em %s: %d novos, %d atualizados, %d iguais, %d mantidos\n"
	msgPromptOverwrite = "O prompt %s já existe e é diferente. Substituir? [s/N] "
	msgPromptKept      = "  mantido sem alterar: %s (rode com --yes para substituir)\n"
	msgSkillWritten    = "✓ skill do Vitral em %s (versão %s)\n"
	msgSkillSame       = "✓ skill do Vitral já está em %s (versão %s)\n"
	msgSkillOverwrite  = "A skill %s (versão %s) foi editada. Substituir pela desta versão (%s)? [s/N] "
	msgSkillKept       = "  skill mantida sem alterar: %s (rode com --yes para substituir)\n"
	msgGlobalPlan      = "Vou configurar o Vitral para todos os projetos (VS Code, todos os perfis, e Copilot CLI):\n  servidor: %s\n"
	msgGlobalPrompts   = "  prompts do %s: %s\n"
	msgGlobalConfirm   = "Confirma? [s/N] "
	msgGlobalSkipped   = "Nada foi alterado.\n"
	msgGlobalNoVSCode  = "  Não encontrei o VS Code neste computador: configurei o servidor e a skill; os prompts ficam para quando ele estiver instalado (rode o setup de novo).\n"
	msgGlobalNeedsTTY  = "o setup --global precisa de confirmação. Rode num terminal ou use --yes para confirmar"
	msgMigrateConfirm  = "O Vitral também está em %s, no formato antigo do VS Code. Tirar de lá? Com ele em dois lugares, o VS Code sobe o Vitral duas vezes. [s/N] "
	msgMigrated        = "✓ tirei o Vitral do formato antigo em %s\n"
	msgMigrateKept     = "  mantido: %s (o Vitral pode subir duas vezes; rode vitral doctor para conferir)\n"
	msgNextStepsLocal  = "\nPróximos passos:\n  1. Abra esta pasta no VS Code.\n  2. Rode o comando \"MCP: List Servers\", escolha \"vitral\" e clique em \"Start\".\n  3. No chat do Copilot, escolha o modo Agent e digite / para ver os prompts do Vitral.\n"
	msgNextStepsGlobal = "\nPróximos passos:\n  1. Reinicie o VS Code.\n  2. Rode o comando \"MCP: List Servers\", escolha \"vitral\" e clique em \"Start\".\n  3. No chat do Copilot, escolha o modo Agent e digite / para ver os prompts do Vitral.\n"

	msgLocWorkspace       = "pasta"
	msgLocWorkspaceGitHub = "pasta, .github"
	msgLocWorkspaceVSCode = "pasta, .vscode"
	msgLocGlobal          = "usuário"
	msgLocVSCodeUser      = "%s, perfil padrão"
	msgLocVSCodeProfile   = "%s, perfil %s"

	msgDisabled       = "✓ Vitral desligado em %s\n"
	msgDisableNothing = "O Vitral não está ligado em nenhum arquivo de configuração (nada mudou).\n"
	msgDisableNext    = "O VS Code para o servidor quando o arquivo muda. Se a janela do Vitral continuar aberta, rode \"MCP: List Servers\", escolha \"vitral\" e \"Stop\".\nPara ligar de novo: vitral enable\n"
	msgEnabled        = "✓ Vitral ligado em %s\n"
	msgEnableNothing  = "Não há Vitral desligado em nenhum arquivo de configuração (nada mudou).\n"
	msgEnableNext     = "No VS Code, rode \"MCP: List Servers\", escolha \"vitral\" e clique em \"Start\" (ou só use o chat).\n"

	msgStatusTitle        = "Vitral %s (%s)\n"
	msgStatusInstalled    = "  instalado em %s (versão %s)\n"
	msgStatusNotInstalled = "  não instalado em %s (rode: vitral setup)\n"
	msgStatusConfig       = "\nConfiguração:\n"
	msgStatusActive       = "  ✓ ligado     %s (%s)\n"
	msgStatusDisabled     = "  – desligado  %s (%s)\n"
	msgStatusUnreadable   = "  ✗ ilegível   %s (%s): %v\n"
	msgStatusDeprecated   = ", formato antigo do VS Code: rode vitral setup para migrar"
	msgStatusNone         = "  nenhum arquivo com o Vitral (rode: vitral setup)\n"
	msgStatusDuplicate    = "  ! ligado em %d lugares: o VS Code pode subir o Vitral mais de uma vez (deixe um só: vitral disable --workspace ou --global)\n"
	msgStatusSkill        = "\nSkill:\n"
	msgStatusSkillAt      = "  ✓ %s (versão %s)\n"
	msgStatusSkillOld     = "  ! %s está na versão %s; o Vitral é a %s (rode: vitral setup)\n"
	msgStatusSkillNone    = "  não instalada (o Vitral funciona sem ela; rode vitral setup para instalar)\n"
	msgStatusBrowser      = "\nNavegador: %s\nPerfil: %s (%s)\n"
	msgStatusSessions     = "Sessões salvas: %d em %s (vitral session list)\n"

	msgMapUsage    = "use: vitral map list | show <site> | export <site> --to arquivo.json | import arquivo.json | clear <site>"
	msgMapNone     = "Nenhum mapa de telas em %s.\n"
	msgMapBroken   = "  %s: arquivo ilegível (%v)\n"
	msgMapItem     = "  %s: %d telas, %d transições\n"
	msgMapNotFound = "não há mapa de %s em %s"
	msgMapMermaid  = "\nGrafo (Mermaid):\n%s"
	msgMapExported = "✓ mapa de %s exportado para %s (só estrutura: papéis, nomes de controles, títulos e padrões de URL)\n"
	msgMapImported = "✓ mapa de %s importado: %d telas, em %s\n"
	msgMapCleared  = "✓ mapa de %s apagado\n"

	msgSessionUsage       = "use: vitral session save <nome> | vitral session list | vitral session delete <nome>"
	msgSessionBadName     = "nome de sessão inválido %q: use letras minúsculas, números, - e _"
	msgSessionNone        = "Nenhuma sessão salva em %s.\n"
	msgSessionBroken      = "  %s: arquivo ilegível (%s)\n"
	msgSessionItem        = "  %s: perfil %s, salva em %s, %d cookies, localStorage de: %s\n"
	msgSessionNoOrigins   = "nenhuma origem"
	msgSessionNotFound    = "não há sessão %q em %s"
	msgSessionDeleted     = "✓ sessão %s apagada\n"
	msgSessionSaved       = "✓ sessão %s salva em %s: %d cookies, localStorage de: %s\n"
	msgSessionWarning     = "  Atenção: este arquivo dá acesso às suas contas. Não compartilhe e nunca coloque no git.\n  IndexedDB e sessionStorage não entram: alguns sites podem pedir login de novo.\n  Para usar: inicie o Vitral com --session <nome>.\n"
	msgSessionProfileBusy = "não consegui abrir o perfil %s (%s): ele deve estar aberto em outro Vitral, como o do VS Code. Peça na conversa para salvar a sessão (ferramenta session_save) ou feche o Vitral e tente de novo"
	msgUnknownCommand     = "comando desconhecido %q. Rode \"vitral\" para ver a ajuda"
	msgUnknownFlag        = "opção desconhecida %q para %s"
	msgDoctorTitle        = "Vitral %s: conferindo se está tudo pronto\n\n"
	msgDoctorOK           = "✓ %s\n"
	msgDoctorFail         = "✗ %s\n  → %s\n"
	msgDoctorAllGood      = "\nTudo pronto.\n"
	msgDoctorSomeFailed   = "\nAlgo precisa de atenção (veja as setas acima).\n"

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
	msgCheckSkillOK       = "skill do Vitral em %s (versão %s)"
	msgCheckSkillMiss     = "a skill do Vitral não está instalada (nem em .github/skills/vitral nem em ~/.copilot/skills/vitral); o Vitral funciona sem ela, mas a IA segue melhor as regras de segurança com ela"
	msgCheckSkillOld      = "a skill em %s está na versão %s, e o Vitral na %s"
	msgCheckMCPOK         = "%s aponta para %s"
	msgCheckMCPMissing    = "%s não tem o servidor \"vitral\""
	msgCheckMCPBadPath    = "%s aponta para %s, que não existe"
	msgCheckMCPOldVersion = "%s aponta para %s (versão %s), mas a instalada é %s"
	msgCheckMCPNone       = "nenhum arquivo de configuração com o Vitral ligado"
	msgFixMCP             = "rode: vitral setup (ou vitral setup --global)"
	msgCheckMCPLegacy     = "%s usa o formato antigo do VS Code (deprecado)"
	msgFixMCPLegacyLocal  = "rode: vitral setup (passa para .mcp.json)"
	msgFixMCPLegacyGlobal = "rode: vitral setup --global (passa para ~/.copilot/mcp-config.json)"
	msgCheckMCPDuplicate  = "o Vitral está ligado em %d arquivos; o VS Code pode subir mais de um"
	msgFixMCPDuplicate    = "deixe um só: vitral disable --workspace (ou --global); veja com vitral status"

	msgDownloadStart    = "Baixando o navegador do Vitral (só na primeira vez)…\n"
	msgDownloadProgress = "Baixando o navegador do Vitral (só na primeira vez)… %s\n"
	msgDownloadUnzip    = "Preparando o navegador…\n"
	msgDownloadDone     = "Navegador pronto.\n"

	answerYes = "s"
)
