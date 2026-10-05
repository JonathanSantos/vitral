package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/config"
)

func TestOpenCommand(t *testing.T) {
	f := newFakeEnv(t, "darwin")
	var gotArgs []string
	load := func(args []string) (config.Config, error) {
		gotArgs = args
		cfg := config.Defaults(f.Home)
		cfg.ProfileDir = config.ProfilePath(f.Home, cfg.Profile)
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--profile":
				cfg.Profile = args[i+1]
				cfg.ProfileDir = config.ProfilePath(f.Home, cfg.Profile)
				i++
			case "--no-retouches":
				cfg.NoRetouches = true
			case "--profile-dir":
				cfg.ProfileDir = args[i+1]
				i++
			case "--bad":
				return cfg, errors.New("bad flag --bad")
			}
		}
		return cfg, nil
	}
	var gotURL string
	var gotCfg config.Config
	stub := func(out openOutcome, err error) {
		openBrowser = func(_ Env, cfg config.Config, url string) (openOutcome, error) {
			gotCfg, gotURL = cfg, url
			return out, err
		}
	}
	defer func(orig func(Env, config.Config, string) (openOutcome, error)) { openBrowser = orig }(openBrowser)

	for _, tc := range []struct {
		args    []string
		out     openOutcome
		err     error
		wantOut string
		wantErr string
		url     string
		flags   []string
		noRet   bool
	}{
		{args: nil, out: openOutcome{Retouches: 2}, wantOut: "✓ navegador do Vitral aberto (perfil default), com 2 retoques\n"},
		{args: []string{"sistema.empresa.com"}, out: openOutcome{Retouches: 1}, url: "https://sistema.empresa.com", wantOut: "(perfil default), com 1 retoque\n"},
		{args: []string{"http://127.0.0.1:8766/x", "--profile", "hml"}, url: "http://127.0.0.1:8766/x", flags: []string{"--profile", "hml"}, wantOut: "aberto (perfil hml)\n"},
		{args: []string{"--sem-retoques", "about:blank"}, url: "about:blank", noRet: true, wantOut: ", sem retoques\n"},
		{args: []string{"--no-retouches", "a.com"}, url: "https://a.com", flags: []string{"--no-retouches"}, noRet: true, wantOut: ", sem retoques\n"},
		{args: []string{"--profile-dir", "/tmp/perfil-x"}, flags: []string{"--profile-dir", "/tmp/perfil-x"}, wantOut: "aberto (perfil /tmp/perfil-x)\n"},
		{args: []string{"--headless", "a.com"}, url: "https://a.com", flags: []string{"--headless"}, wantOut: "aberto (perfil default)\n"},
		{args: []string{"--profile=hml", "chrome://settings"}, url: "chrome://settings", flags: []string{"--profile=hml"}, wantOut: "aberto"},
		{out: openOutcome{OpenResult: browser.OpenResult{NewWindow: true}}, wantOut: "a IA já está usando o perfil default: abri uma janela nova no navegador dela"},
		{out: openOutcome{OpenResult: browser.OpenResult{Handoff: true}}, wantOut: "já estava aberto no perfil default"},
		{out: openOutcome{OpenResult: browser.OpenResult{Headless: true}}, wantErr: "a IA está usando o perfil default sem janela agora"},
		{err: &browser.OpenError{Err: errors.New("exit status 21"), Busy: true}, wantErr: "o perfil default já está aberto em outro navegador"},
		{err: errors.New("no browser"), wantErr: "não consegui abrir o navegador: no browser"},
		{args: []string{"a.com", "b.com"}, wantErr: "uso: vitral open"},
		{args: []string{"--bad"}, flags: []string{"--bad"}, wantErr: "bad flag --bad"},
	} {
		f.out.Reset()
		gotURL, gotCfg, gotArgs = "", config.Config{}, nil
		stub(tc.out, tc.err)
		err := OpenCommand(f.Env, tc.args, load)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%v: error %v, want %q", tc.args, err, tc.wantErr)
			}
			continue
		}
		if err != nil || !strings.Contains(f.out.String(), tc.wantOut) {
			t.Errorf("%v: %v %q, want %q", tc.args, err, f.out.String(), tc.wantOut)
			continue
		}
		if gotURL != tc.url || !reflect.DeepEqual(gotArgs, tc.flags) && (len(gotArgs) > 0 || len(tc.flags) > 0) || gotCfg.NoRetouches != tc.noRet {
			t.Errorf("%v: url %q args %v noRetouches %v; want %q %v %v", tc.args, gotURL, gotArgs, gotCfg.NoRetouches, tc.url, tc.flags, tc.noRet)
		}
	}
}

func TestTakesValue(t *testing.T) {
	for flag, want := range map[string]bool{"--profile": true, "--retouches-dir": true, "--headless": false, "-no-map": false, "--no-retouches": false} {
		if got := takesValue(flag); got != want {
			t.Errorf("takesValue(%q) = %v", flag, got)
		}
	}
}
