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
	TextOverlayName = "Vitral"

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
)

// Label é o texto de status do selo para o estado.
func Label(s State) string {
	switch s.Mode {
	case Working:
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

// ButtonLabel é o texto do botão de pausa.
func ButtonLabel(s State) string {
	if s.Blocked() {
		return TextResume
	}
	return TextPause
}
