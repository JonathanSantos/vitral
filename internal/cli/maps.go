package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/mapstore"
)

// MapCommand é o "vitral map list|show|export|import|clear" (pedido 5.2 e 5.4). O mapa guarda só
// estrutura (papéis, nomes de controles, títulos, padrões de URL), nunca dados da tela.
func MapCommand(e Env, args []string, loadConfig func([]string) (config.Config, error)) error {
	if len(args) == 0 {
		return errors.New(msgMapUsage)
	}
	sub, rest := args[0], args[1:]
	arg, to := "", ""
	switch sub {
	case "list":
	case "show", "clear", "import":
		if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
			return errors.New(msgMapUsage)
		}
		arg, rest = rest[0], rest[1:]
	case "export":
		if len(rest) < 3 || rest[1] != "--to" {
			return errors.New(msgMapUsage)
		}
		arg, to, rest = rest[0], rest[2], rest[3:]
	default:
		return errors.New(msgMapUsage)
	}
	cfg, err := loadConfig(rest)
	if err != nil {
		return err
	}
	st := mapstore.Store{Dir: cfg.MapsDir}
	switch sub {
	case "list":
		entries, err := os.ReadDir(cfg.MapsDir)
		var hosts []string
		for _, en := range entries {
			if name, ok := strings.CutSuffix(en.Name(), ".json"); ok && !en.IsDir() {
				hosts = append(hosts, name)
			}
		}
		if err != nil || len(hosts) == 0 {
			fmt.Fprintf(e.Stdout, msgMapNone, cfg.MapsDir)
			return nil
		}
		sort.Strings(hosts)
		for _, h := range hosts {
			data, err := os.ReadFile(st.Path(h))
			if err != nil {
				continue
			}
			m, err := mapstore.Decode(data)
			if err != nil {
				fmt.Fprintf(e.Stdout, msgMapBroken, h, err)
				continue
			}
			fmt.Fprintf(e.Stdout, msgMapItem, m.Host, len(m.Screens), len(m.Edges))
		}
		return nil
	case "show":
		m, err := st.Load(arg)
		if err != nil {
			return err
		}
		if len(m.Screens) == 0 {
			return fmt.Errorf(msgMapNotFound, arg, cfg.MapsDir)
		}
		fmt.Fprint(e.Stdout, m.Text())
		fmt.Fprintf(e.Stdout, msgMapMermaid, m.Mermaid())
		return nil
	case "export":
		if err := st.Export(arg, to); err != nil {
			return err
		}
		fmt.Fprintf(e.Stdout, msgMapExported, arg, to)
		return nil
	case "import":
		m, err := st.Import(arg)
		if err != nil {
			return err
		}
		fmt.Fprintf(e.Stdout, msgMapImported, m.Host, len(m.Screens), st.Path(m.Host))
		return nil
	}
	if err := st.Clear(arg); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, msgMapCleared, arg)
	return nil
}
