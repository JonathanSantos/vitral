package console

import (
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

func TestBufferEvents(t *testing.T) {
	b := NewBuffer(3)
	frame := &proto.RuntimeStackTrace{CallFrames: []*proto.RuntimeCallFrame{{URL: "http://x/app.js", LineNumber: 9}}}
	b.OnConsole(&proto.RuntimeConsoleAPICalled{
		Type:       proto.RuntimeConsoleAPICalledTypeLog,
		Args:       []*proto.RuntimeRemoteObject{{Type: "string", Value: gson.New("loaded")}, {Type: "number", Value: gson.New(2), Description: "2"}},
		StackTrace: frame,
	})
	b.OnConsole(&proto.RuntimeConsoleAPICalled{Type: proto.RuntimeConsoleAPICalledTypeWarning, Args: []*proto.RuntimeRemoteObject{{Type: "string", Value: gson.New("careful")}}})
	b.OnException(&proto.RuntimeExceptionThrown{ExceptionDetails: &proto.RuntimeExceptionDetails{
		Text: "Uncaught", URL: "http://x/app.js", LineNumber: 41,
		Exception: &proto.RuntimeRemoteObject{Description: "TypeError: x is null\n    at foo (app.js:42)"},
	}})
	level := proto.LogLogEntryLevelError
	line := 0
	b.OnLog(&proto.LogEntryAdded{Entry: &proto.LogLogEntry{Level: level, Text: "Failed to load resource", URL: "http://x/api", LineNumber: &line}})

	msgs := b.Messages()
	if len(msgs) != 3 {
		t.Fatalf("ring should keep 3, got %d", len(msgs))
	}
	want := []string{
		"[warn] careful",
		"[error] Uncaught TypeError: x is null (http://x/app.js:42)",
		"[error] Failed to load resource (http://x/api:1)",
	}
	for i, w := range want {
		if got := Line(msgs[i]); got != w {
			t.Errorf("line %d = %q want %q", i, got, w)
		}
	}
	if b.Mark() != 4 {
		t.Fatalf("mark = %d", b.Mark())
	}
}

func TestFilterAndLevels(t *testing.T) {
	msgs := []Message{{ID: 1, Level: "log"}, {ID: 2, Level: "warn"}, {ID: 3, Level: "error"}, {ID: 4, Level: "error"}}
	tests := []struct {
		level string
		limit int
		want  int
	}{
		{"all", 0, 4}, {"", 0, 4}, {"error", 0, 2}, {"warn", 0, 3}, {"all", 1, 1}, {"log", 0, 1},
	}
	for _, tt := range tests {
		if got := Filter(msgs, tt.level, tt.limit); len(got) != tt.want {
			t.Errorf("Filter(%q,%d) = %d msgs, want %d", tt.level, tt.limit, len(got), tt.want)
		}
	}
	if got := Filter(msgs, "all", 1); got[0].ID != 4 {
		t.Error("limit should keep the newest")
	}
	levels := map[string]string{"warning": "warn", "assert": "error", "verbose": "debug", "dir": "log", "info": "info"}
	for in, want := range levels {
		if got := NormalizeLevel(in); got != want {
			t.Errorf("NormalizeLevel(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRemoteObjectText(t *testing.T) {
	tests := []struct {
		obj  *proto.RuntimeRemoteObject
		want string
	}{
		{&proto.RuntimeRemoteObject{Type: "string", Value: gson.New("oi")}, "oi"},
		{&proto.RuntimeRemoteObject{Type: "object", Description: "Object"}, "Object"},
		{&proto.RuntimeRemoteObject{Type: "boolean", Value: gson.New(true)}, "true"},
		{&proto.RuntimeRemoteObject{Type: "number", UnserializableValue: "NaN"}, "NaN"},
		{&proto.RuntimeRemoteObject{Type: "undefined"}, "undefined"},
	}
	for _, tt := range tests {
		if got := RemoteObjectText(tt.obj); got != tt.want {
			t.Errorf("RemoteObjectText(%+v)=%q want %q", tt.obj, got, tt.want)
		}
	}
}

func TestBriefIsShort(t *testing.T) {
	m := Message{
		Level: "error", Text: "Failed to load resource: " + strings.Repeat("y", 300),
		Source: "https://rr3.googlevideo.com/videoplayback?expire=1&sig=" + strings.Repeat("z", 500),
	}
	got := Brief(m)
	if !strings.HasPrefix(got, "[error] Failed to load resource: ") || !strings.HasSuffix(got, "(https://rr3.googlevideo.com/videoplayback?…)") {
		t.Fatalf("got %q", got)
	}
	if n := len([]rune(got)); n > 200 {
		t.Fatalf("brief line has %d chars", n)
	}
	long := Message{Level: "warning", Text: "ok", Source: "https://example.com/" + strings.Repeat("p", 120)}
	if got := Brief(long); !strings.HasSuffix(got, "…)") {
		t.Fatalf("long source should be cut: %q", got)
	}
	if got := Brief(Message{Level: "log", Text: "hi"}); got != "[log] hi" {
		t.Fatalf("no source: %q", got)
	}
}
