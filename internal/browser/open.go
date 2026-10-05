package browser

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/config"
)

// vitral open (docs/RETOQUES.md, 2.3 e decisão 14.1): o navegador do perfil para a pessoa usar, sem as
// bandeiras de automação e sem porta nem pipe de depuração (os sites o veem como um Chrome comum), com
// os retoques. O perfil é o mesmo da IA, mas os dois não abrem ao mesmo tempo.

// OpenResult diz o que o vitral open fez.
type OpenResult struct {
	PID       int  // navegador lançado (0 quando a janela foi aberta no navegador da IA)
	NewWindow bool // a IA já estava com o perfil aberto: a janela nova é no navegador dela
	Headless  bool // a IA está com o perfil aberto sem janela: nada visível pôde ser aberto
	Handoff   bool // outro navegador do vitral open já tinha o perfil e recebeu o pedido
}

// earlyExit é quanto o vitral open espera para saber se o navegador abriu ou saiu na hora.
var earlyExit = 1500 * time.Millisecond

// OpenForPerson abre o navegador do perfil (vitral open). bin é o executável já resolvido; extDirs são
// as extensões prontas (os Retoques); url pode ser vazia.
func OpenForPerson(cfg config.Config, bin, url string, extDirs []string) (OpenResult, error) {
	profile := cfg.ProfileDir
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return OpenResult{}, fmt.Errorf("create profile dir: %w", err)
	}
	if b, headless, ok := debuggable(profile); ok {
		defer func() { _ = b.Close() }()
		if headless {
			return OpenResult{Headless: true}, nil
		}
		target := url
		if target == "" {
			target = "chrome://newtab/"
		}
		if _, err := (proto.TargetCreateTarget{URL: target, NewWindow: true}).Call(b); err != nil {
			return OpenResult{}, fmt.Errorf("open a window in the browser already using the profile: %w", err)
		}
		return OpenResult{NewWindow: true}, nil
	}

	logPath := filepath.Join(profile, "vitral-open.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return OpenResult{}, err
	}
	defer logFile.Close()
	mock := UseMockKeychain(cfg, bin, profile, nil)
	cmd := exec.Command(bin, OpenArgs(cfg, profile, url, extDirs, mock)...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return OpenResult{}, fmt.Errorf("start browser: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err == nil {
			// Saiu sem erro logo de cara: outro navegador com este perfil recebeu o pedido.
			return OpenResult{Handoff: true}, nil
		}
		out, _ := os.ReadFile(logPath)
		return OpenResult{}, &OpenError{Err: err, Log: lastLines(out, 5), Busy: busyProfile(string(out))}
	case <-time.After(earlyExit):
		return OpenResult{PID: cmd.Process.Pid}, nil
	}
}

// OpenArgs são as bandeiras do vitral open: o mínimo, sem as de automação que o rod põe por padrão.
// mockKeychain vem de UseMockKeychain, a mesma regra da sessão da IA (decisão 14.2): os dois modos
// precisam da mesma chave, ou os logins de um não abrem no outro.
func OpenArgs(cfg config.Config, profile, url string, extDirs []string, mockKeychain bool) []string {
	args := []string{
		"--user-data-dir=" + profile,
		"--no-first-run",
		"--no-default-browser-check",
	}
	if mockKeychain {
		args = append(args, "--use-mock-keychain")
	}
	if len(extDirs) > 0 {
		args = append(args, "--load-extension="+strings.Join(extDirs, ","))
	}
	if cfg.Lang != "" {
		args = append(args, "--lang="+cfg.Lang)
	}
	if cfg.Headless { // só para os testes: a pessoa sempre tem janela
		args = append(args, "--headless")
	}
	if url != "" {
		args = append(args, url)
	}
	return args
}

// OpenError é a falha do vitral open, com o fim do log do navegador para explicar.
type OpenError struct {
	Err  error
	Log  string
	Busy bool // o perfil está em uso por outro navegador
}

func (e *OpenError) Error() string {
	if e.Log == "" {
		return e.Err.Error()
	}
	return e.Err.Error() + ": " + e.Log
}

func (e *OpenError) Unwrap() error { return e.Err }

// debuggable acha o navegador da IA neste perfil pelo DevToolsActivePort (um arquivo velho de uma
// sessão que já fechou não conecta).
func debuggable(profile string) (*rod.Browser, bool, bool) {
	data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	if err != nil {
		return nil, false, false
	}
	port, path, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
	if port == "" || path == "" {
		return nil, false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b := rod.New().ControlURL("ws://127.0.0.1:" + strings.TrimSpace(port) + strings.TrimSpace(path)).Context(ctx)
	if err := b.Connect(); err != nil {
		return nil, false, false
	}
	v, err := proto.BrowserGetVersion{}.Call(b)
	if err != nil {
		_ = b.Close()
		return nil, false, false
	}
	return b.Context(context.Background()), strings.Contains(v.UserAgent, "HeadlessChrome"), true
}

// busyProfile reconhece, no log do Chrome, o perfil já aberto por outro processo.
func busyProfile(log string) bool {
	return strings.Contains(log, "SingletonLock") || strings.Contains(log, "ProcessSingleton") ||
		strings.Contains(log, "profile appears to be in use")
}

func lastLines(out []byte, n int) string {
	lines := strings.Split(strings.TrimSpace(string(bytes.ReplaceAll(out, []byte("\r"), nil))), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, " | "))
}

// ProfileBusyError é o erro para a IA quando o lançamento falha com o perfil em uso: diz se é outro
// Vitral (navegador com depuração) ou a pessoa no vitral open.
func ProfileBusyError(profile string, launchErr error) error {
	if b, _, ok := debuggable(profile); ok {
		_ = b.Close()
		return fmt.Errorf("launch browser: %w. Another %s (another VS Code window or chat) is using the profile %s: close it, use another --profile, or run with --session", launchErr, config.Name, profile)
	}
	return fmt.Errorf("the person is using the profile %s in vitral open (a browser window outside the AI): ask them, in one sentence and in their language, to close that browser window, then call this tool again. Or start %s with another --profile", profile, config.Name)
}
