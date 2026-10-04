package ux

import "fmt"

// Textos para pessoas (PT-BR), centralizados para facilitar tradução futura (docs/BRAND.md, seção 7).
const (
	TextIdle        = "Vitral está usando esta janela"
	TextWorking     = "A IA está trabalhando…"
	TextFlow        = "Executando fluxo: %s · passo %d de %d"
	TextPaused      = "Pausado. Você está no controle."
	TextWaiting     = "Sua vez: %s"
	TextPause       = "⏸ Pausar IA"
	TextResume      = "▶ Retomar"
	TextCollapse    = "Recolher o selo do Vitral"
	TextExpand      = "Vitral: mostrar o selo"
	ReasonPassword  = "preencha a senha e clique em Retomar"
	ReasonCaptcha   = "resolva a verificação e clique em Retomar"
	ReasonCode      = "digite o código e clique em Retomar"
	TextOverlayName = "Vitral"
	TextProfileTag  = "perfil %s"
	TextSessionTag  = "sessão %s"

	// Cursor visual (docs/CURSOR.md).
	TextCursorLabel = "IA"
	TextSelected    = "Selecionado: %s"
	TextCovered     = "Coberto por um diálogo"
	TextDisabled    = "Botão desabilitado"
	TextOpening     = "Abrindo %s"
	TextGoingBack   = "Voltando"
)

// Mensagens para a IA (inglês, imperativo), devolvidas como resultado não-erro.
const (
	PausedMessage     = `Paused by the user: they are using the browser window. Do not retry now. Tell the user, in their language, to click "Retomar" in the browser when they are done, then call snapshot.`
	PasswordMessage   = `Waiting for the user: element %s is a password field, and password fields must be filled by the user in the browser window. The browser is paused with the field highlighted. Do not retry now. Tell the user, in their language, to type the password in the browser window and click "Retomar" when done, then call snapshot.`
	FlowPausedMessage = `Flow %s stopped before step %d of %d: paused by the user. ` + PausedMessage
	CaptchaMessage    = `Waiting for the user: the page shows a %s verification (CAPTCHA). Only the user can solve it, in the browser window; never try to solve or skip it. The browser is paused. Do not retry now. Tell the user, in their language, to solve it and click "Retomar" when done, then call snapshot.`
	CodeMessage       = `Waiting for the user: element %s asks for a one-time code (2FA). The user types it in the browser window; never type a code yourself. The browser is paused with the field highlighted. Do not retry now. Tell the user, in their language, to type the code and click "Retomar" when done, then call snapshot.`
)

// Label é o texto de status do selo para o estado.
func Label(s State) string {
	switch s.Mode {
	case Working:
		if s.Note != "" {
			return s.Note
		}
		return TextWorking
	case Flow:
		return fmt.Sprintf(TextFlow, s.FlowName, s.Step, s.Total)
	case Paused:
		return TextPaused
	case Waiting:
		return fmt.Sprintf(TextWaiting, s.Reason)
	}
	return TextIdle
}

// Tagged junta ao rótulo o perfil ou a sessão em uso, quando não é o padrão ("… · perfil hml").
func Tagged(label, tag string) string {
	if tag == "" {
		return label
	}
	return label + " · " + tag
}

// Tag é a etiqueta do perfil não padrão ou da sessão (--session) para o selo; "" no perfil padrão.
func Tag(profile, session string) string {
	switch {
	case session != "":
		return fmt.Sprintf(TextSessionTag, session)
	case profile != "" && profile != "default":
		return fmt.Sprintf(TextProfileTag, profile)
	}
	return ""
}

// ButtonLabel é o texto do botão de pausa.
func ButtonLabel(s State) string {
	if s.Blocked() {
		return TextResume
	}
	return TextPause
}
