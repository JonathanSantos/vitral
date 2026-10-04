package session

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/input"

	"github.com/JonathanSantos/vitral/internal/heal"
	"github.com/JonathanSantos/vitral/internal/sitemap"
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

func TestScreenMapHelpers(t *testing.T) {
	if got := withScreenLine("page: x\nviewport: y", `screen: "A" (known)`); got != "page: x\nscreen: \"A\" (known)\nviewport: y" {
		t.Fatal(got)
	}
	if withScreenLine("page: x", "s") != "page: x\ns" || withScreenLine("page: x", "") != "page: x" {
		t.Fatal("edges")
	}
	if replaceFirstLine("a\nb", "c") != "c\nb" || replaceFirstLine("a", "c") != "c" {
		t.Fatal("replaceFirstLine")
	}
	sig := &heal.Signature{Role: "link", Name: "Pedidos"}
	if st := gotoStep(sitemap.Action{Kind: "click", Target: sig}); st.Click == nil || st.Click.Name != "Pedidos" || st.Click.Sig == sig {
		t.Fatalf("%+v", st)
	}
	if st := gotoStep(sitemap.Action{Kind: "press", Key: "Escape"}); st.Press != "Escape" {
		t.Fatal("press")
	}
	if st := gotoStep(sitemap.Action{Kind: "back"}); !st.Back {
		t.Fatal("back")
	}
	mp := sitemap.New("p.test")
	if screenNames(mp) != "none yet" {
		t.Fatal("empty")
	}
	now := time.Now()
	a := mp.Visit(sitemap.Observe("https://p.test/a", "", "A", "", []sitemap.Item{{Role: "heading", Name: "A"}}), now)
	b := mp.Visit(sitemap.Observe("https://p.test/b", "", "B", "", []sitemap.Item{{Role: "heading", Name: "B"}}), now)
	c := mp.Visit(sitemap.Observe("https://p.test/c", "", "C", "", []sitemap.Item{{Role: "heading", Name: "C"}}), now)
	mp.Transition(a, b, sitemap.Action{Kind: "click", Target: sig}, now)
	if screenNames(mp) != `"A", "B", "C"` {
		t.Fatal(screenNames(mp))
	}
	for _, tt := range []struct {
		src, dst *sitemap.Screen
		want     string
	}{
		{nil, b, "not in the map yet"}, {a, a, "you are on it"}, {a, b, `1 step(s) from here: click link "Pedidos"`}, {a, c, "no known way from here"},
	} {
		if got := describePath(mp, tt.src, tt.dst); !strings.Contains(got, tt.want) {
			t.Errorf("%v: %s", tt.want, got)
		}
	}
}
