// Package ux guarda o estado do Vitral visto pela pessoa (selo e pausa) como uma máquina de
// estados pura. A página só renderiza o que este pacote decide (docs/UX-TASKS.md, UX-2 e UX-3).
package ux

// Mode é o modo do selo.
type Mode string

const (
	Idle    Mode = "idle"
	Working Mode = "working"
	Flow    Mode = "flow"
	Paused  Mode = "paused"
	Waiting Mode = "waiting" // aguardando a pessoa (ex.: digitar a senha)
)

// Box é a caixa de um elemento em pixels CSS da viewport.
type Box [4]int

// Highlight é um destaque que fica na tela até a pessoa retomar (usado no "Sua vez").
type Highlight struct {
	Ref string `json:"ref"`
	Box Box    `json:"box"`
}

// State é tudo o que o selo precisa para se desenhar.
type State struct {
	Mode      Mode       `json:"mode"`
	FlowName  string     `json:"flowName,omitempty"`
	Step      int        `json:"step,omitempty"`
	Total     int        `json:"total,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	Collapsed bool       `json:"collapsed"`
	Hold      *Highlight `json:"hold,omitempty"`
	Note      string     `json:"note,omitempty"` // rótulo temporário enquanto trabalha ("Abrindo …")
	busy      int        // ferramentas em execução (o servidor pode atender chamadas em paralelo)
}

// EventKind identifica o que aconteceu.
type EventKind string

const (
	ToolStarted  EventKind = "tool_started"
	ToolFinished EventKind = "tool_finished"
	FlowStep     EventKind = "flow_step"
	FlowEnded    EventKind = "flow_ended"
	UserPause    EventKind = "pause"
	UserResume   EventKind = "resume"
	UserToggle   EventKind = "toggle_pause" // o mesmo botão pausa e retoma
	WaitUser     EventKind = "wait_user"
	Collapse     EventKind = "collapse"
	Expand       EventKind = "expand"
	NavStarted   EventKind = "nav_started" // Reason é o rótulo ("Abrindo exemplo.com", "Voltando")
	NavEnded     EventKind = "nav_ended"
)

// Event é uma entrada da máquina de estados.
type Event struct {
	Kind     EventKind
	FlowName string
	Step     int
	Total    int
	Reason   string
	Hold     *Highlight
}

// Blocked indica que as ferramentas que agem não podem executar.
func (s State) Blocked() bool { return s.Mode == Paused || s.Mode == Waiting }

// Next é a função de transição. Pura: não toca em nada fora do estado.
func Next(s State, e Event) State {
	switch e.Kind {
	case Collapse:
		s.Collapsed = true
		return s
	case Expand:
		s.Collapsed = false
		return s
	case ToolStarted:
		s.busy++
		if s.Mode == Idle {
			s.Mode = Working
		}
		return s
	case ToolFinished:
		s.busy = max(s.busy-1, 0)
		if s.busy == 0 {
			s.Note = ""
		}
		if s.busy == 0 && (s.Mode == Working || s.Mode == Flow) {
			s = clearFlow(s)
			s.Mode = Idle
		}
		return s
	case NavStarted:
		if !s.Blocked() {
			s.Note = e.Reason
		}
		return s
	case NavEnded:
		s.Note = ""
		return s
	case FlowStep:
		if s.Blocked() {
			return s
		}
		s.Mode, s.FlowName, s.Step, s.Total = Flow, e.FlowName, e.Step, e.Total
		return s
	case FlowEnded:
		if s.Mode == Flow {
			s = clearFlow(s)
			s.Mode = Working
		}
		return s
	case UserPause:
		if s.Blocked() {
			return s
		}
		s = clearFlow(s)
		s.Mode = Paused
		return s
	case UserResume:
		if !s.Blocked() {
			return s
		}
		return resume(s)
	case UserToggle:
		if s.Blocked() {
			return resume(s)
		}
		return Next(s, Event{Kind: UserPause})
	case WaitUser:
		if s.Mode == Paused {
			return s
		}
		s = clearFlow(s)
		s.Mode, s.Reason, s.Hold = Waiting, e.Reason, e.Hold
		return s
	}
	return s
}

// resume volta a trabalhar se ainda há ferramenta rodando, senão fica ocioso.
func resume(s State) State {
	s.Reason, s.Hold = "", nil
	s.Mode = Idle
	if s.busy > 0 {
		s.Mode = Working
	}
	return s
}

func clearFlow(s State) State {
	s.FlowName, s.Step, s.Total = "", 0, 0
	return s
}
