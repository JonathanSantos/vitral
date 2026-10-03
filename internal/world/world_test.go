package world

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

func TestNoPageAttached(t *testing.T) {
	tr := New()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := tr.Call(ctx, `() => 1`); err == nil {
		t.Fatal("call without page must fail")
	}
	if err := tr.Eval(ctx, `1`); err == nil {
		t.Fatal("eval without page must fail")
	}
}

func TestContextEvents(t *testing.T) {
	tr := New()
	tr.frame = "main"
	ctxFor := func(name, frame string, id int) *proto.RuntimeExecutionContextCreated {
		return &proto.RuntimeExecutionContextCreated{Context: &proto.RuntimeExecutionContextDescription{
			ID: proto.RuntimeExecutionContextID(id), Name: name,
			AuxData: map[string]gson.JSON{"frameId": gson.New(frame)},
		}}
	}
	tests := []struct {
		name  string
		event func()
		want  proto.RuntimeExecutionContextID
	}{
		{"page world is ignored", func() { tr.onCreated(ctxFor("", "main", 1)) }, 0},
		{"iframe world is ignored", func() { tr.onCreated(ctxFor(Name, "child", 2)) }, 0},
		{"main frame vitral world", func() { tr.onCreated(ctxFor(Name, "main", 3)) }, 3},
		{"other context destroyed", func() { tr.onDestroyed(&proto.RuntimeExecutionContextDestroyed{ExecutionContextID: 9}) }, 3},
		{"our context destroyed", func() { tr.onDestroyed(&proto.RuntimeExecutionContextDestroyed{ExecutionContextID: 3}) }, 0},
		{"new document", func() { tr.onCreated(ctxFor(Name, "main", 4)) }, 4},
		{"contexts cleared", func() { tr.onCleared(nil) }, 0},
		{"stale id forgotten", func() { tr.onCreated(ctxFor(Name, "main", 5)); tr.forget(5) }, 0},
	}
	for _, tt := range tests {
		tt.event()
		if tr.id != tt.want {
			t.Fatalf("%s: id = %d, want %d", tt.name, tr.id, tt.want)
		}
	}
}

func TestStaleContextAndExceptionText(t *testing.T) {
	if !staleContext(errors.New("{-32000 Cannot find context with specified id}")) ||
		!staleContext(errors.New("Execution context was destroyed.")) ||
		staleContext(errors.New("other")) || staleContext(nil) {
		t.Fatal("staleContext")
	}
	d := &proto.RuntimeExceptionDetails{Text: "Uncaught", Exception: &proto.RuntimeRemoteObject{Description: "TypeError: x\n    at y"}}
	if exceptionText(d) != "TypeError: x" {
		t.Fatalf("got %q", exceptionText(d))
	}
	if exceptionText(&proto.RuntimeExceptionDetails{Text: "Uncaught"}) != "Uncaught" {
		t.Fatal("fallback text")
	}
}
