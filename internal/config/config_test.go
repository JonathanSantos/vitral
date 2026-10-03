package config

import (
	"io/fs"
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

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		files   map[string]string
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
		{name: "attach conflict", args: []string{"--attach", "ws://x", "--attach-port", "9222"}, wantErr: true},
		{name: "bad window", args: []string{"--window-size", "big"}, wantErr: true},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Load(tt.args, "/home/u", fakeFS(tt.files))
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
