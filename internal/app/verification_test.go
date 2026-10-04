//go:build integration

package app_test

import (
	"strings"
	"testing"
)

// Verificação humana (pedido 4.3), sem janela (o harness é headless): CAPTCHA e código de uso único
// viram falha acionável; nada é digitado nem contornado. As fixtures imitam a estrutura dos serviços.
func TestVerificationHeadless(t *testing.T) {
	for _, tt := range []struct{ page, what string }{
		{"recaptcha.html", "a reCAPTCHA check (CAPTCHA)"},
		{"hcaptcha.html", "a hCaptcha check (CAPTCHA)"},
		{"turnstile.html", "a Cloudflare Turnstile check (CAPTCHA)"},
	} {
		t.Run(tt.page, func(t *testing.T) {
			h.fail(t, "open", map[string]any{"url": h.url("/pages/verify/" + tt.page)},
				"verification required: run with the browser window visible. The page asks for "+tt.what)
		})
	}
	t.Run("invisible captcha is not a hand-off", func(t *testing.T) {
		mustContain(t, h.ok(t, "open", map[string]any{"url": h.url("/pages/verify/recaptcha-invisivel.html")}), `heading "Entrar"`)
	})
	t.Run("captcha that appears after an action", func(t *testing.T) {
		snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/verify/escondido.html")})
		h.fail(t, "click", map[string]any{"ref": ref(t, snap, "button", "Entrar")}, "The page asks for a reCAPTCHA check")
	})
	t.Run("one-time code field", func(t *testing.T) {
		snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/verify/codigo.html")})
		h.fail(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Código"), "text": "123456"}, "The page asks for a one-time code (2FA)")
		mustContain(t, h.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Apelido"), "text": "ana"}), `value="ana"`)
		if got := h.mainWorld(t, `() => ({v: document.querySelector('[name=otp]').value})`)["v"]; got != "" {
			t.Fatalf("the code field must stay empty: %q", got)
		}
		if strings.Contains(h.ok(t, "snapshot", nil), "123456") {
			t.Fatal("the code must not be typed")
		}
	})
}
