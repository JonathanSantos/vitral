package config

import (
	"io/fs"
	"strings"
	"testing"
	"time"
)

func fakeFS(files map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		data, ok := files[p]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return []byte(data), nil
	}
}

func TestHighlight(t *testing.T) {
	tests := []struct {
		cfg  Config
		want time.Duration
	}{
		{Config{HighlightMs: -1}, 350 * time.Millisecond},
		{Config{HighlightMs: -1, Headless: true}, 0},
		{Config{HighlightMs: 200, Headless: true}, 200 * time.Millisecond},
		{Config{HighlightMs: 0}, 0},
		{Config{HighlightMs: 500, NoOverlay: true}, 0},
	}
	for _, tt := range tests {
		if got := tt.cfg.Highlight(); got != tt.want {
			t.Errorf("%+v: got %v want %v", tt.cfg, got, tt.want)
		}
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		files   map[string]string
		env     map[string]string
		check   func(t *testing.T, c Config)
		wantErr bool
	}{
		{
			name: "defaults without file",
			check: func(t *testing.T, c Config) {
				if c.ProfileDir != "/home/u/.vitral/profile" || c.NavTimeout != 15*time.Second || c.NetworkBuffer != 500 {
					t.Fatalf("unexpected defaults: %+v", c)
				}
			},
		},
		{
			name:  "yaml then flags win",
			args:  []string{"--network-buffer", "50"},
			files: map[string]string{DefaultConfigFile: "headless: true\nnetworkBuffer: 10\nallowedHosts: [a.com]\n"},
			check: func(t *testing.T, c Config) {
				if !c.Headless || c.NetworkBuffer != 50 || len(c.AllowedHosts) != 1 {
					t.Fatalf("unexpected merge: %+v", c)
				}
			},
		},
		{
			name:  "explicit config path",
			args:  []string{"--config=custom.yaml"},
			files: map[string]string{"custom.yaml": "allowEval: true"},
			check: func(t *testing.T, c Config) {
				if !c.AllowEval {
					t.Fatal("expected allowEval from custom file")
				}
			},
		},
		{name: "explicit config missing", args: []string{"--config", "nope.yaml"}, wantErr: true},
		{
			name: "list flags and home expansion",
			args: []string{"--allowed-hosts", "a.com, *.b.com", "--profile-dir", "~/p"},
			check: func(t *testing.T, c Config) {
				if len(c.AllowedHosts) != 2 || c.AllowedHosts[1] != "*.b.com" || c.ProfileDir != "/home/u/p" {
					t.Fatalf("unexpected: %+v", c)
				}
			},
		},
		{
			name: "env then flags",
			args: []string{"--browser-mirror", "https://flag/"},
			env:  map[string]string{EnvBrowserPath: "~/chrome/chrome", EnvBrowserMirror: "https://env/"},
			check: func(t *testing.T, c Config) {
				if c.BrowserPath != "/home/u/chrome/chrome" || c.BrowserMirror != "https://flag/" || c.BrowsersDir != "/home/u/.vitral/browsers" {
					t.Fatalf("unexpected: %+v", c)
				}
			},
		},
		{
			name:  "system browser opt-in by yaml",
			files: map[string]string{DefaultConfigFile: "allowSystemBrowser: true"},
			check: func(t *testing.T, c Config) {
				if !c.AllowSystemBrowser {
					t.Fatal("allowSystemBrowser from yaml")
				}
			},
		},
		{name: "attach conflict", args: []string{"--attach", "ws://x", "--attach-port", "9222"}, wantErr: true},
		{name: "bad window", args: []string{"--window-size", "big"}, wantErr: true},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Load(tt.args, "/home/u", fakeFS(tt.files), func(k string) string { return tt.env[k] })
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, c)
		})
	}
}

func TestCursorConfig(t *testing.T) {
	load := func(args ...string) (Config, error) {
		return Load(args, "/home/u", func(string) ([]byte, error) { return nil, fs.ErrNotExist }, nil)
	}
	cfg, err := load()
	if err != nil || cfg.Cursor != "on" || cfg.CursorSpeed != "normal" || cfg.FlowCursorSpeed != "fast" || cfg.TypePacing != "on" || cfg.CursorSeed != 0 {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	if !cfg.CursorActive() || !cfg.TypePacingActive() {
		t.Fatal("visible window: cursor and pacing on")
	}
	tests := []struct {
		args           []string
		cursor, pacing bool
	}{
		{[]string{"--headless"}, false, false},
		{[]string{"--headless", "--cursor", "force"}, true, true},
		{[]string{"--cursor", "off"}, false, true},
		{[]string{"--no-overlay"}, false, true},
		{[]string{"--no-overlay", "--cursor", "force"}, false, true},
		{[]string{"--type-pacing", "off"}, true, false},
	}
	for _, tt := range tests {
		cfg, err := load(tt.args...)
		if err != nil || cfg.CursorActive() != tt.cursor || cfg.TypePacingActive() != tt.pacing {
			t.Errorf("%v: cursor=%v pacing=%v err=%v", tt.args, cfg.CursorActive(), cfg.TypePacingActive(), err)
		}
	}
	cfg, err = load("--cursor-speed", "slow", "--flow-cursor-speed", "instant", "--cursor-seed", "42")
	if err != nil || cfg.CursorSpeed != "slow" || cfg.FlowCursorSpeed != "instant" || cfg.CursorSeed != 42 {
		t.Fatalf("flags: %+v %v", cfg, err)
	}
	for _, bad := range [][]string{{"--cursor", "maybe"}, {"--cursor-speed", "warp"}, {"--flow-cursor-speed", "x"}, {"--type-pacing", "yes"}} {
		if _, err := load(bad...); err == nil || !strings.Contains(err.Error(), bad[0]) {
			t.Errorf("%v should fail naming the flag: %v", bad, err)
		}
	}
}
