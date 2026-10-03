package session

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// ResponseBody busca o corpo de uma resposta sob demanda (Network.getResponseBody).
func (s *Session) ResponseBody(ctx context.Context, id proto.NetworkRequestID) ([]byte, error) {
	var body []byte
	err := s.WithPage(func(p *rod.Page) error {
		res, err := proto.NetworkGetResponseBody{RequestID: id}.Call(p.Context(ctx))
		if err != nil {
			return fmt.Errorf("response body not available (page navigated or the browser discarded it): %w", err)
		}
		if !res.Base64Encoded {
			body = []byte(res.Body)
			return nil
		}
		body, err = base64.StdEncoding.DecodeString(res.Body)
		return err
	})
	return body, err
}
