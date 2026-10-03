package session

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod/lib/input"
)

var namedKeys = map[string]input.Key{
	"enter": input.Enter, "return": input.Enter, "escape": input.Escape, "esc": input.Escape,
	"tab": input.Tab, "backspace": input.Backspace, "delete": input.Delete, "space": input.Space,
	"arrowup": input.ArrowUp, "arrowdown": input.ArrowDown, "arrowleft": input.ArrowLeft, "arrowright": input.ArrowRight,
	"up": input.ArrowUp, "down": input.ArrowDown, "left": input.ArrowLeft, "right": input.ArrowRight,
	"home": input.Home, "end": input.End, "pageup": input.PageUp, "pagedown": input.PageDown,
	"f1": input.F1, "f2": input.F2, "f3": input.F3, "f4": input.F4, "f5": input.F5, "f6": input.F6,
	"f7": input.F7, "f8": input.F8, "f9": input.F9, "f10": input.F10, "f11": input.F11, "f12": input.F12,
}

var modifierKeys = map[string]input.Key{
	"control": input.ControlLeft, "ctrl": input.ControlLeft, "shift": input.ShiftLeft,
	"alt": input.AltLeft, "option": input.AltLeft, "meta": input.MetaLeft, "cmd": input.MetaLeft, "command": input.MetaLeft,
}

// ParseKeys converte "Control+Shift+K" em modificadores e tecla final.
func ParseKeys(combo string) ([]input.Key, input.Key, error) {
	parts := strings.Split(strings.TrimSpace(combo), "+")
	// "+" sozinho ou no fim ("Control++") é a própria tecla +.
	if strings.HasSuffix(combo, "++") || combo == "+" {
		parts = append(strings.Split(strings.TrimSuffix(combo, "++"), "+"), "+")
		if combo == "+" {
			parts = []string{"+"}
		}
	}
	var mods []input.Key
	for _, m := range parts[:len(parts)-1] {
		key, ok := modifierKeys[strings.ToLower(strings.TrimSpace(m))]
		if !ok {
			return nil, 0, fmt.Errorf("unknown modifier %q. Use Control, Shift, Alt or Meta.", m)
		}
		mods = append(mods, key)
	}
	key, err := parseKey(parts[len(parts)-1])
	return mods, key, err
}

func parseKey(name string) (input.Key, error) {
	if k, ok := namedKeys[strings.ToLower(strings.TrimSpace(name))]; ok {
		return k, nil
	}
	runes := []rune(name)
	if len(runes) == 1 && runes[0] >= ' ' && runes[0] <= '~' {
		return input.Key(runes[0]), nil
	}
	return 0, fmt.Errorf("unknown key %q. Use names like Enter, Escape, Tab, ArrowDown, or a single character.", name)
}
