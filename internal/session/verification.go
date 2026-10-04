package session

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/JonathanSantos/vitral/internal/ux"
)

// Verificação humana (pedido 4.3): CAPTCHA e código de uso único são com a pessoa. Com a janela
// visível, o Vitral entrega a vez ("Sua vez"); sem janela (headless), não há quem resolva: falha
// acionável. Nada de contornar: o Vitral só reconhece a verificação e para.

//go:embed verification.json
var verificationJSON []byte

type detectors struct {
	Captcha []struct {
		Name     string `json:"name"`
		Selector string `json:"selector"`
	} `json:"captcha"`
	Ignore []string `json:"ignore"`
	Code   string   `json:"code"`
}

var verification = func() detectors {
	var d detectors
	if err := json.Unmarshal(verificationJSON, &d); err != nil {
		panic("verification.json: " + err.Error())
	}
	return d
}()

// VerificationRequired é o erro de uma verificação humana sem janela visível.
func verificationRequired(what string) error {
	return fmt.Errorf("verification required: run with the browser window visible. The page asks for %s, which only the user can do: start Vitral without --headless and let the user do it (or log in by hand once and save the session with session_save)", what)
}

// handOff decide o que fazer com uma verificação: pausar para a pessoa (janela visível) ou falhar.
func handOff(headless bool, what string) (pause bool, err error) {
	if headless {
		return false, verificationRequired(what)
	}
	return true, nil
}

type captchaHit struct {
	Name string `json:"name"`
	Box  [4]int `json:"box"`
}

// captchaOnPage procura um CAPTCHA visível (iframes conhecidos, inclusive em shadow roots abertos).
func (s *Session) captchaOnPage(ctx context.Context) (captchaHit, bool) {
	var hit captchaHit
	js := `(list, ignore) => {
		const roots = [document];
		for (let i = 0; i < roots.length; i++) for (const el of roots[i].querySelectorAll('*')) if (el.shadowRoot) roots.push(el.shadowRoot);
		for (const d of list) for (const root of roots) for (const el of root.querySelectorAll(d.selector)) {
			const src = el.getAttribute('src') || '';
			if (ignore.some((s) => src.includes(s))) continue;
			const r = el.getBoundingClientRect();
			const vis = el.checkVisibility ? el.checkVisibility({checkOpacity: true, checkVisibilityCSS: true}) : true;
			if (vis && r.width >= 30 && r.height >= 30) return {name: d.name, box: [Math.round(r.left), Math.round(r.top), Math.round(r.width), Math.round(r.height)]};
		}
		return {name: ''};
	}`
	if err := s.evalInto(ctx, &hit, js, verification.Captcha, verification.Ignore); err != nil || hit.Name == "" {
		return hit, false
	}
	return hit, true
}

// verificationGate roda depois de cada ação: com um CAPTCHA na tela, pausa para a pessoa (ou falha
// sem janela). Devolve a mensagem para a IA ("" sem verificação).
func (s *Session) verificationGate(ctx context.Context) (string, error) {
	hit, ok := s.captchaOnPage(ctx)
	if !ok {
		return "", nil
	}
	if _, err := handOff(s.cfg.Headless, "a "+hit.Name+" check (CAPTCHA)"); err != nil {
		return "", err
	}
	s.ui.WaitForUser(ux.ReasonCaptcha, "", hit.Box)
	return fmt.Sprintf(ux.CaptchaMessage, hit.Name), nil
}
