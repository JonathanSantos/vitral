// Package console guarda mensagens de console e exceções da página num ring buffer.
package console

import (
	"fmt"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/ring"
)

// Message é uma mensagem de console, exceção ou log do browser.
type Message struct {
	ID     int
	Level  string // error, warn, info, log, debug
	Text   string
	Source string // arquivo:linha
}

// Buffer recebe eventos de Runtime e Log.
type Buffer struct {
	mu     sync.Mutex
	buf    *ring.Buffer[Message]
	nextID int
}

// NewBuffer cria um buffer com capacidade fixa.
func NewBuffer(capacity int) *Buffer {
	return &Buffer{buf: ring.New[Message](capacity)}
}

// Attach liga o buffer a uma página nova.
func (b *Buffer) Attach(p *rod.Page) error {
	if err := (proto.RuntimeEnable{}).Call(p); err != nil {
		return err
	}
	if err := (proto.LogEnable{}).Call(p); err != nil {
		return err
	}
	go p.EachEvent(b.OnConsole, b.OnException, b.OnLog)()
	return nil
}

func (b *Buffer) add(level, text, source string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	b.buf.Push(Message{ID: b.nextID, Level: level, Text: text, Source: source})
}

// OnConsole trata Runtime.consoleAPICalled.
func (b *Buffer) OnConsole(e *proto.RuntimeConsoleAPICalled) {
	parts := make([]string, 0, len(e.Args))
	for _, a := range e.Args {
		parts = append(parts, RemoteObjectText(a))
	}
	b.add(NormalizeLevel(string(e.Type)), strings.Join(parts, " "), stackSource(e.StackTrace))
}

// OnException trata Runtime.exceptionThrown (exceções não tratadas).
func (b *Buffer) OnException(e *proto.RuntimeExceptionThrown) {
	d := e.ExceptionDetails
	if d == nil {
		return
	}
	text := d.Text
	if d.Exception != nil && d.Exception.Description != "" {
		text = strings.TrimSpace(text + " " + firstLine(d.Exception.Description))
	}
	source := stackSource(d.StackTrace)
	if source == "" && d.URL != "" {
		source = fmt.Sprintf("%s:%d", d.URL, d.LineNumber+1)
	}
	b.add("error", text, source)
}

// OnLog trata Log.entryAdded (erros de rede, violações, intervenções do browser).
func (b *Buffer) OnLog(e *proto.LogEntryAdded) {
	if e.Entry == nil {
		return
	}
	source := e.Entry.URL
	if e.Entry.LineNumber != nil {
		source = fmt.Sprintf("%s:%d", source, *e.Entry.LineNumber+1)
	}
	b.add(NormalizeLevel(string(e.Entry.Level)), e.Entry.Text, source)
}

// Mark devolve o id da última mensagem.
func (b *Buffer) Mark() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nextID
}

// Messages devolve cópias das mensagens, da mais antiga para a mais nova.
func (b *Buffer) Messages() []Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Items()
}

// NormalizeLevel reduz os níveis do CDP a error/warn/info/log/debug.
func NormalizeLevel(level string) string {
	switch strings.ToLower(level) {
	case "error", "assert":
		return "error"
	case "warning", "warn":
		return "warn"
	case "info":
		return "info"
	case "debug", "verbose", "trace":
		return "debug"
	}
	return "log"
}

// Filter devolve as últimas limit mensagens do nível pedido ("error", "warn" inclui error, "all").
func Filter(msgs []Message, level string, limit int) []Message {
	var out []Message
	for _, m := range msgs {
		if matchLevel(level, m.Level) {
			out = append(out, m)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func matchLevel(filter, level string) bool {
	switch strings.ToLower(filter) {
	case "", "all":
		return true
	case "error", "errors":
		return level == "error"
	case "warn", "warning":
		return level == "error" || level == "warn"
	}
	return strings.EqualFold(filter, level)
}

// Line formata uma mensagem: [level] texto (origem).
func Line(m Message) string {
	text := m.Text
	if len([]rune(text)) > 300 {
		text = string([]rune(text)[:299]) + "…"
	}
	line := fmt.Sprintf("[%s] %s", m.Level, text)
	if m.Source != "" {
		line += " (" + m.Source + ")"
	}
	return line
}

// Brief é a linha curta usada no resumo de uma ação: texto até 120 caracteres e a origem sem query
// (a mensagem completa está no console_list).
func Brief(m Message) string {
	text := m.Text
	if len([]rune(text)) > 120 {
		text = string([]rune(text)[:119]) + "…"
	}
	line := fmt.Sprintf("[%s] %s", m.Level, text)
	if src := m.Source; src != "" {
		base, query, hasQuery := strings.Cut(src, "?")
		if hasQuery && query != "" {
			base += "?…"
		}
		if r := []rune(base); len(r) > 80 {
			base = string(r[:79]) + "…"
		}
		line += " (" + base + ")"
	}
	return line
}

// RemoteObjectText transforma um argumento de console em texto legível.
func RemoteObjectText(o *proto.RuntimeRemoteObject) string {
	if o == nil {
		return ""
	}
	if o.Type == proto.RuntimeRemoteObjectTypeString {
		return o.Value.Str()
	}
	if o.Description != "" {
		return o.Description
	}
	if o.UnserializableValue != "" {
		return string(o.UnserializableValue)
	}
	raw := o.Value.JSON("", "")
	if raw != "" && raw != "null" || o.Subtype == proto.RuntimeRemoteObjectSubtypeNull {
		return raw
	}
	return string(o.Type)
}

func stackSource(st *proto.RuntimeStackTrace) string {
	if st == nil || len(st.CallFrames) == 0 {
		return ""
	}
	f := st.CallFrames[0]
	if f.URL == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", f.URL, f.LineNumber+1)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
