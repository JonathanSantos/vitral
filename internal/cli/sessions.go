package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/storagestate"
)

// SessionCommand é o "vitral session save|list|delete" (pedido 4.2). O conteúdo das sessões nunca é
// mostrado: só nome, perfil, data, quantidade de cookies e origens.
func SessionCommand(e Env, args []string, loadConfig func([]string) (config.Config, error)) error {
	if len(args) == 0 {
		return errors.New(msgSessionUsage)
	}
	sub, rest := args[0], args[1:]
	if sub != "save" && sub != "list" && sub != "delete" {
		return errors.New(msgSessionUsage)
	}
	name := ""
	if sub == "save" || sub == "delete" {
		if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
			return errors.New(msgSessionUsage)
		}
		name, rest = rest[0], rest[1:]
		if !config.ValidName(name) {
			return fmt.Errorf(msgSessionBadName, name)
		}
	}
	cfg, err := loadConfig(rest)
	if err != nil {
		return err
	}
	store := storagestate.Store{Dir: cfg.SessionsDir}
	switch sub {
	case "list":
		list, err := store.List()
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Fprintf(e.Stdout, msgSessionNone, cfg.SessionsDir)
			return nil
		}
		for _, s := range list {
			if s.Created.IsZero() {
				fmt.Fprintf(e.Stdout, msgSessionBroken, s.Name, s.Path)
				continue
			}
			fmt.Fprintf(e.Stdout, msgSessionItem, s.Name, s.Profile, s.Created.Local().Format("02/01/2006 15:04"), s.Cookies, originsText(s.Origins))
		}
		return nil
	case "delete":
		if err := store.Delete(name); err != nil {
			return fmt.Errorf(msgSessionNotFound, name, cfg.SessionsDir)
		}
		fmt.Fprintf(e.Stdout, msgSessionDeleted, name)
		return nil
	}
	return sessionSave(e, cfg, name)
}

// sessionSave abre o perfil sem janela e salva o estado. O Chrome não abre o mesmo perfil duas vezes:
// com o Vitral aberto no VS Code, o caminho é a ferramenta session_save na conversa.
func sessionSave(e Env, cfg config.Config, name string) error {
	cfg.Headless, cfg.Session, cfg.Attach, cfg.AttachPort = true, "", "", 0
	m := browser.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer m.Close()
	sum, err := m.SaveState(name)
	if err != nil {
		if strings.Contains(err.Error(), "launch browser") || strings.Contains(err.Error(), "in vitral open") {
			return fmt.Errorf(msgSessionProfileBusy, cfg.Profile, cfg.ProfileDir)
		}
		return err
	}
	fmt.Fprintf(e.Stdout, msgSessionSaved, sum.Name, sum.Path, sum.Cookies, originsText(sum.Origins))
	fmt.Fprint(e.Stdout, msgSessionWarning)
	return nil
}

func originsText(origins []string) string {
	if len(origins) == 0 {
		return msgSessionNoOrigins
	}
	return strings.Join(origins, ", ")
}
