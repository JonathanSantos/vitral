package session

import (
	"slices"
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/input"
)

func TestParseKeys(t *testing.T) {
	tests := []struct {
		combo   string
		mods    []input.Key
		key     input.Key
		wantErr bool
	}{
		{combo: "Enter", key: input.Enter},
		{combo: "escape", key: input.Escape},
		{combo: "Control+A", mods: []input.Key{input.ControlLeft}, key: input.Key('A')},
		{combo: "Ctrl+Shift+k", mods: []input.Key{input.ControlLeft, input.ShiftLeft}, key: input.Key('k')},
		{combo: "+", key: input.Key('+')},
		{combo: "Control++", mods: []input.Key{input.ControlLeft}, key: input.Key('+')},
		{combo: "Hyper+A", wantErr: true},
		{combo: "Banana", wantErr: true},
		{combo: "ç", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.combo, func(t *testing.T) {
			mods, key, err := ParseKeys(tt.combo)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if key != tt.key || !slices.Equal(mods, tt.mods) {
				t.Fatalf("got mods=%v key=%v", mods, key)
			}
		})
	}
}

func TestHandOff(t *testing.T) {
	if pause, err := handOff(false, "x"); !pause || err != nil {
		t.Fatalf("visible window: %v %v", pause, err)
	}
	pause, err := handOff(true, "a one-time code (2FA)")
	if pause || err == nil || !strings.HasPrefix(err.Error(), "verification required: run with the browser window visible. The page asks for a one-time code (2FA)") {
		t.Fatalf("headless: %v %v", pause, err)
	}
	if len(verification.Captcha) != 3 || verification.Code == "" || len(verification.Ignore) == 0 {
		t.Fatalf("detectors: %+v", verification)
	}
}
