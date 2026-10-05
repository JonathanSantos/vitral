package cli

import (
	"fmt"

	"github.com/JonathanSantos/vitral/internal/config"
)

// Main trata os comandos para pessoas. handled=false significa "rode o servidor MCP".
// Sem argumentos e com terminal, mostra a ajuda em vez de esperar JSON-RPC para sempre.
func Main(args []string, e Env, loadConfig func([]string) (config.Config, error)) (handled bool, code int) {
	if len(args) == 0 {
		if e.Interactive {
			fmt.Fprint(e.Stdout, msgHelp)
			return true, 0
		}
		return false, 0
	}
	switch args[0] {
	case "version", "--version", "-version":
		fmt.Fprintf(e.Stdout, msgVersion, config.Name, config.Version)
		return true, 0
	case "help", "--help", "-h", "-help":
		fmt.Fprint(e.Stdout, msgHelp)
		return true, 0
	case "setup":
		opts, err := parseSetup(args[1:])
		if err == nil {
			err = Setup(e, opts)
		}
		return true, report(e, err)
	case "install-browser":
		zipPath, rest, err := parseInstallBrowser(args[1:])
		if err != nil {
			return true, report(e, err)
		}
		cfg, err := loadConfig(rest)
		if err == nil {
			err = InstallBrowser(e, cfg, zipPath)
		}
		return true, report(e, err)
	case "disable", "enable":
		opts, err := parseScope(args[0], args[1:])
		if err == nil {
			if args[0] == "disable" {
				err = Disable(e, opts)
			} else {
				err = Enable(e, opts)
			}
		}
		return true, report(e, err)
	case "map":
		return true, report(e, MapCommand(e, args[1:], loadConfig))
	case "session":
		return true, report(e, SessionCommand(e, args[1:], loadConfig))
	case "open":
		return true, report(e, OpenCommand(e, args[1:], loadConfig))
	case "status":
		cfg, err := loadConfig(args[1:])
		if err != nil {
			return true, report(e, err)
		}
		Status(e, cfg)
		return true, 0
	case "doctor":
		cfg, err := loadConfig(args[1:])
		if err != nil {
			return true, report(e, err)
		}
		if !Doctor(e, cfg) {
			return true, 1
		}
		return true, 0
	}
	if args[0][0] != '-' {
		return true, report(e, fmt.Errorf(msgUnknownCommand, args[0]))
	}
	return false, 0
}

func report(e Env, err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(e.Stderr, "%s: %v\n", config.Name, err)
	return 1
}
