package cli

import (
	"fmt"
)

// ScopeOptions escolhem onde o disable/enable age: na pasta, no usuário ou nos dois (padrão).
type ScopeOptions struct {
	Workspace, Global bool
}

func parseScope(cmd string, args []string) (ScopeOptions, error) {
	var o ScopeOptions
	for _, a := range args {
		switch a {
		case "--workspace", "-workspace":
			o.Workspace = true
		case "--global", "-global":
			o.Global = true
		default:
			return o, fmt.Errorf(msgUnknownFlag, a, cmd)
		}
	}
	if !o.Workspace && !o.Global {
		o.Workspace, o.Global = true, true
	}
	return o, nil
}

func (o ScopeOptions) locations(e Env) []Location {
	var out []Location
	if o.Workspace {
		out = append(out, WorkspaceLocations(e)...)
	}
	if o.Global {
		out = append(out, GlobalLocations(e)...)
	}
	return out
}

// Disable desliga o Vitral em todos os arquivos onde ele está ativo: a entrada vai para a chave
// "_disabled" (não existe campo oficial de "desligado"). Rodar de novo não muda nada.
func Disable(e Env, o ScopeOptions) error {
	changed := 0
	for _, p := range Inspect(o.locations(e)) {
		if p.State != StateActive {
			continue
		}
		if _, err := moveIn(e, p.Path, p.Key, KeyDisabled); err != nil {
			return err
		}
		changed++
		fmt.Fprintf(e.Stdout, msgDisabled, p.Path)
	}
	if changed == 0 {
		fmt.Fprint(e.Stdout, msgDisableNothing)
		return nil
	}
	fmt.Fprint(e.Stdout, msgDisableNext)
	return nil
}

// Enable devolve a entrada desligada para a lista de servidores.
func Enable(e Env, o ScopeOptions) error {
	changed := 0
	for _, p := range Inspect(o.locations(e)) {
		if p.State != StateDisabled {
			continue
		}
		if _, err := moveIn(e, p.Path, KeyDisabled, p.Key); err != nil {
			return err
		}
		changed++
		fmt.Fprintf(e.Stdout, msgEnabled, p.Path)
	}
	if changed == 0 {
		fmt.Fprint(e.Stdout, msgEnableNothing)
		return nil
	}
	fmt.Fprint(e.Stdout, msgEnableNext)
	return nil
}
