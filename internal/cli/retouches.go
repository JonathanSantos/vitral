package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/retouch"
)

// RetouchCommand é o "vitral retoque list|delete <nome>" (docs/RETOQUES.md, 3.5): a pessoa vê e apaga os
// retoques sem a IA. Apagar nunca é uma ferramenta da IA.
func RetouchCommand(e Env, args []string, loadConfig func([]string) (config.Config, error)) error {
	if len(args) == 0 || (args[0] != "list" && args[0] != "delete") {
		return errors.New(msgRetouchUsage)
	}
	sub, rest := args[0], args[1:]
	name := ""
	if sub == "delete" {
		if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
			return errors.New(msgRetouchUsage)
		}
		name, rest = rest[0], rest[1:]
	}
	cfg, err := loadConfig(rest)
	if err != nil {
		return err
	}
	store := retouch.Store{Dir: cfg.RetouchesDir}
	if sub == "delete" {
		if err := store.Delete(name); err != nil {
			return fmt.Errorf(msgRetouchNotFound, name, cfg.RetouchesDir)
		}
		// A extensão gerada é refeita no próximo lançamento (ou no próximo save).
		fmt.Fprintf(e.Stdout, msgRetouchDeleted, name)
		return nil
	}
	list, bad, err := store.List()
	if err != nil {
		return err
	}
	if len(list) == 0 && len(bad) == 0 {
		fmt.Fprintf(e.Stdout, msgRetouchNone, cfg.RetouchesDir)
		return nil
	}
	for _, r := range list {
		desc := ""
		if r.Description != "" {
			desc = " · " + r.Description
		}
		fmt.Fprintf(e.Stdout, msgRetouchItem, r.Name, strings.Join(r.Matches, ", "), desc)
	}
	names := make([]string, 0, len(bad))
	for n := range bad {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(e.Stdout, msgRetouchBroken, n, bad[n])
	}
	return nil
}
