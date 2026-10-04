package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/config"
)

// Status mostra onde o Vitral está configurado (e se está ativo), as versões do binário e da skill
// e o navegador que ele vai usar. Só lê: não baixa nem abre nada.
func Status(e Env, cfg config.Config) {
	fmt.Fprintf(e.Stdout, msgStatusTitle, config.Version, e.Executable)
	installed := InstalledPath(e)
	if _, err := os.Stat(installed); err == nil {
		fmt.Fprintf(e.Stdout, msgStatusInstalled, installed, binaryVersion(e, installed))
	} else {
		fmt.Fprintf(e.Stdout, msgStatusNotInstalled, installed)
	}

	fmt.Fprint(e.Stdout, msgStatusConfig)
	places := Inspect(append(WorkspaceLocations(e), GlobalLocations(e)...))
	active := 0
	for _, p := range places {
		label := p.Label
		if p.Deprecated {
			label += msgStatusDeprecated
		}
		switch {
		case p.Err != nil:
			fmt.Fprintf(e.Stdout, msgStatusUnreadable, p.Path, label, p.Err)
		case p.State == StateActive:
			active++
			fmt.Fprintf(e.Stdout, msgStatusActive, p.Path, label)
		case p.State == StateDisabled:
			fmt.Fprintf(e.Stdout, msgStatusDisabled, p.Path, label)
		}
	}
	switch {
	case active == 0 && !hasState(places, StateDisabled):
		fmt.Fprint(e.Stdout, msgStatusNone)
	case active > 1:
		fmt.Fprintf(e.Stdout, msgStatusDuplicate, active)
	}

	fmt.Fprint(e.Stdout, msgStatusSkill)
	found := false
	for _, dir := range []string{WorkspaceSkillDir(e.Cwd), GlobalSkillDir(e.Home)} {
		target := filepath.Join(dir, "SKILL.md")
		if data, err := os.ReadFile(target); err == nil {
			v := SkillVersion(data)
			if v == config.Version {
				fmt.Fprintf(e.Stdout, msgStatusSkillAt, target, v)
			} else {
				fmt.Fprintf(e.Stdout, msgStatusSkillOld, target, v, config.Version)
			}
			found = true
		}
	}
	if !found {
		fmt.Fprint(e.Stdout, msgStatusSkillNone)
	}

	res, resErr := browser.Resolve(cfg, nil, true)
	fmt.Fprintf(e.Stdout, msgStatusBrowser, checkBrowser(cfg, res, resErr).Text, cfg.ProfileDir)
}

func hasState(places []Placement, state string) bool {
	for _, p := range places {
		if p.State == state {
			return true
		}
	}
	return false
}
