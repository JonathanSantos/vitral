package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/jors/browser-mcp/internal/network"
)

// Eval avalia uma expressão JS na página e devolve o resultado serializado (só com --allow-eval).
func (s *Session) Eval(ctx context.Context, expression string) (string, error) {
	if expression == "" {
		return "", errors.New("expression is required")
	}
	var out string
	err := s.WithPage(func(p *rod.Page) error {
		res, err := proto.RuntimeEvaluate{
			Expression:    expression,
			ReturnByValue: true,
			AwaitPromise:  true,
			UserGesture:   true,
		}.Call(p.Context(ctx))
		if err != nil {
			return fmt.Errorf("evaluate: %w", err)
		}
		if res.ExceptionDetails != nil {
			msg := res.ExceptionDetails.Text
			if ex := res.ExceptionDetails.Exception; ex != nil && ex.Description != "" {
				msg = ex.Description
			}
			return fmt.Errorf("javascript error: %s", msg)
		}
		out = describeResult(res.Result)
		return nil
	})
	return out, err
}

func describeResult(r *proto.RuntimeRemoteObject) string {
	if r == nil || r.Type == proto.RuntimeRemoteObjectTypeUndefined {
		return "undefined"
	}
	if r.UnserializableValue != "" {
		return string(r.UnserializableValue)
	}
	raw := r.Value.JSON("", "")
	if raw == "" || raw == "null" && r.Subtype != proto.RuntimeRemoteObjectSubtypeNull {
		return r.Description
	}
	return network.FormatBody([]byte(raw), "application/json")
}
