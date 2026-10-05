package cli

import (
	"errors"
	"fmt"
	"strings"

	vitral "github.com/JonathanSantos/vitral"
	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/retouch"
)

// openOutcome é o que o lançamento devolve para a mensagem.
type openOutcome struct {
	browser.OpenResult
	Retouches int
}

// openBrowser abre o navegador do perfil para a pessoa (trocado nos testes).
var openBrowser = func(e Env, cfg config.Config, url string) (openOutcome, error) {
	res, err := browser.Resolve(cfg, DownloadLogger{W: e.Stderr}, false)
	if err != nil {
		return openOutcome{}, err
	}
	var out openOutcome
	var dirs []string
	if !cfg.NoRetouches {
		b := retouch.Builder{Store: retouch.Store{Dir: cfg.RetouchesDir}, Icons: retouch.IconsFrom(vitral.ExtensionIcons, "assets/brand")}
		if dirs, err = b.Launch(); err != nil {
			return openOutcome{}, err
		}
		if len(dirs) > 0 {
			list, _, _ := b.Store.List()
			out.Retouches = len(list)
		}
	}
	out.OpenResult, err = browser.OpenForPerson(cfg, res.Path, url, dirs)
	return out, err
}

// OpenCommand é o "vitral open [endereço] [--sem-retoques] [--profile p]" (docs/RETOQUES.md, 2.3):
// abre o navegador do Vitral para a pessoa usar, com os retoques e os logins do perfil, sem nenhum
// aviso (decisão 12.1).
func OpenCommand(e Env, args []string, loadConfig func([]string) (config.Config, error)) error {
	url, noRetouches := "", false
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--sem-retoques":
			noRetouches = true
		case strings.HasPrefix(a, "-"):
			rest = append(rest, a)
			// Bandeiras com valor separado (--profile hml): o valor vai junto.
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && takesValue(a) {
				i++
				rest = append(rest, args[i])
			}
		case url == "":
			url = withScheme(a)
		default:
			return errors.New(msgOpenUsage)
		}
	}
	cfg, err := loadConfig(rest)
	if err != nil {
		return err
	}
	cfg.NoRetouches = cfg.NoRetouches || noRetouches
	out, err := openBrowser(e, cfg, url)
	var oerr *browser.OpenError
	switch {
	case errors.As(err, &oerr) && oerr.Busy:
		return fmt.Errorf(msgOpenBusy, cfg.Profile)
	case err != nil:
		return fmt.Errorf(msgOpenFailed, err)
	case out.Headless:
		return fmt.Errorf(msgOpenHeadless, cfg.Profile)
	case out.NewWindow:
		fmt.Fprintf(e.Stdout, msgOpenNewWindow, cfg.Profile)
	case out.Handoff:
		fmt.Fprintf(e.Stdout, msgOpenHandoff, cfg.Profile)
	case cfg.NoRetouches:
		fmt.Fprintf(e.Stdout, msgOpenStarted, cfg.Profile, msgOpenNoRetouches)
	case out.Retouches == 1:
		fmt.Fprintf(e.Stdout, msgOpenStarted, cfg.Profile, msgOpenOneRetouch)
	case out.Retouches > 1:
		fmt.Fprintf(e.Stdout, msgOpenStarted, cfg.Profile, fmt.Sprintf(msgOpenRetouches, out.Retouches))
	default:
		fmt.Fprintf(e.Stdout, msgOpenStarted, cfg.Profile, "")
	}
	return nil
}

// takesValue diz se a bandeira do Vitral recebe um valor (as booleanas não).
func takesValue(flag string) bool {
	switch strings.TrimLeft(flag, "-") {
	case "headless", "no-retouches", "no-map", "allow-system-browser", "allow-eval", "disable-mock", "no-overlay", "preserve-network":
		return false
	}
	return true
}

// withScheme completa "sistema.empresa.com" para https://sistema.empresa.com.
func withScheme(url string) string {
	if strings.Contains(url, "://") || strings.HasPrefix(url, "about:") || strings.HasPrefix(url, "chrome:") {
		return url
	}
	return "https://" + url
}
