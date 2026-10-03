// Package config lê flags de CLI e o arquivo opcional vitral.yaml.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Name é o nome do produto. Centralizado aqui porque ainda pode mudar.
const Name = "vitral"

// Version é sobrescrita no build via -ldflags.
var Version = "dev"

// Config reúne todas as opções do servidor.
type Config struct {
	Headless   bool   `yaml:"headless"`
	Attach     string `yaml:"attach"`
	AttachPort int    `yaml:"attachPort"`
	ProfileDir string `yaml:"profileDir"`
	BrowserBin string `yaml:"browserBin"`
	WindowSize string `yaml:"windowSize"`

	AllowEval    bool     `yaml:"allowEval"`
	AllowedHosts []string `yaml:"allowedHosts"`
	DisableMock  bool     `yaml:"disableMock"`
	MaskHeaders  []string `yaml:"maskHeaders"`

	NavTimeout       time.Duration `yaml:"navTimeout"`
	StabilityTimeout time.Duration `yaml:"stabilityTimeout"`

	NetworkBuffer   int  `yaml:"networkBuffer"`
	PreserveNetwork bool `yaml:"preserveNetwork"`
	ConsoleBuffer   int  `yaml:"consoleBuffer"`

	FlowsDir string `yaml:"flowsDir"`
	LogLevel string `yaml:"logLevel"`
}

// Defaults devolve a configuração padrão. homeDir vem de fora para manter a função pura.
func Defaults(homeDir string) Config {
	return Config{
		ProfileDir:       filepath.Join(homeDir, "."+Name, "profile"),
		WindowSize:       "1280,800",
		NavTimeout:       15 * time.Second,
		StabilityTimeout: 10 * time.Second,
		NetworkBuffer:    500,
		ConsoleBuffer:    200,
		FlowsDir:         filepath.Join("."+Name, "flows"),
		LogLevel:         "info",
	}
}

// DefaultConfigFile é o arquivo lido automaticamente quando existe no diretório de trabalho.
const DefaultConfigFile = Name + ".yaml"

// Load aplica, nesta ordem: defaults, arquivo yaml e flags (flags vencem).
func Load(args []string, homeDir string, readFile func(string) ([]byte, error)) (Config, error) {
	cfg := Defaults(homeDir)

	path, explicit := configPath(args)
	data, err := readFile(path)
	if err != nil && (explicit || !errors.Is(err, fs.ErrNotExist)) {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	if err := parseFlags(&cfg, args); err != nil {
		return cfg, err
	}
	cfg.ProfileDir = expandHome(cfg.ProfileDir, homeDir)
	return cfg, validate(cfg)
}

// configPath acha --config antes do parse completo, porque o yaml define os defaults das flags.
func configPath(args []string) (string, bool) {
	for i, a := range args {
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if name != "config" || !strings.HasPrefix(a, "-") {
			continue
		}
		if hasValue {
			return value, true
		}
		if i+1 < len(args) {
			return args[i+1], true
		}
	}
	return DefaultConfigFile, false
}

func parseFlags(cfg *Config, args []string) error {
	fset := flag.NewFlagSet(Name, flag.ContinueOnError)
	fset.SetOutput(io.Discard)

	fset.String("config", DefaultConfigFile, "path to the yaml config file")
	fset.BoolVar(&cfg.Headless, "headless", cfg.Headless, "run the browser without a window")
	fset.StringVar(&cfg.Attach, "attach", cfg.Attach, "attach to a running Chrome by websocket URL (ws://...)")
	fset.IntVar(&cfg.AttachPort, "attach-port", cfg.AttachPort, "attach to a running Chrome by remote debugging port")
	fset.StringVar(&cfg.ProfileDir, "profile-dir", cfg.ProfileDir, "dedicated browser profile directory")
	fset.StringVar(&cfg.BrowserBin, "browser-bin", cfg.BrowserBin, "path to the Chrome/Chromium binary")
	fset.StringVar(&cfg.WindowSize, "window-size", cfg.WindowSize, "browser window size as W,H")
	fset.BoolVar(&cfg.AllowEval, "allow-eval", cfg.AllowEval, "register the eval_js tool")
	fset.Func("allowed-hosts", "comma separated host globs allowed for navigation and mocks", func(v string) error {
		cfg.AllowedHosts = splitList(v)
		return nil
	})
	fset.BoolVar(&cfg.DisableMock, "disable-mock", cfg.DisableMock, "disable the mock tools")
	fset.Func("mask-headers", "extra comma separated header names to mask", func(v string) error {
		cfg.MaskHeaders = splitList(v)
		return nil
	})
	fset.DurationVar(&cfg.NavTimeout, "nav-timeout", cfg.NavTimeout, "navigation timeout")
	fset.DurationVar(&cfg.StabilityTimeout, "stability-timeout", cfg.StabilityTimeout, "max wait for the page to settle after actions")
	fset.IntVar(&cfg.NetworkBuffer, "network-buffer", cfg.NetworkBuffer, "max requests kept in history")
	fset.BoolVar(&cfg.PreserveNetwork, "preserve-network", cfg.PreserveNetwork, "keep network history across navigations")
	fset.IntVar(&cfg.ConsoleBuffer, "console-buffer", cfg.ConsoleBuffer, "max console messages kept")
	fset.StringVar(&cfg.FlowsDir, "flows-dir", cfg.FlowsDir, "directory for saved flows")
	fset.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "debug, info, warn or error")

	if err := fset.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, usage(fset))
	}
	if fset.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fset.Args())
	}
	return nil
}

func usage(fset *flag.FlagSet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "usage: %s [flags]\n", Name)
	fset.SetOutput(&b)
	fset.PrintDefaults()
	fset.SetOutput(io.Discard)
	return b.String()
}

func validate(cfg Config) error {
	if cfg.Attach != "" && cfg.AttachPort != 0 {
		return errors.New("use either --attach or --attach-port, not both")
	}
	if cfg.Attach != "" && !strings.HasPrefix(cfg.Attach, "ws://") && !strings.HasPrefix(cfg.Attach, "http://") {
		return fmt.Errorf("--attach must be a ws:// or http:// URL, got %q", cfg.Attach)
	}
	if cfg.NetworkBuffer < 1 || cfg.ConsoleBuffer < 1 {
		return errors.New("buffer sizes must be positive")
	}
	if _, _, err := ParseWindowSize(cfg.WindowSize); err != nil {
		return err
	}
	return nil
}

// ParseWindowSize converte "W,H" (ou "WxH") em inteiros.
func ParseWindowSize(s string) (int, int, error) {
	var w, h int
	normalized := strings.ReplaceAll(strings.ToLower(s), "x", ",")
	if _, err := fmt.Sscanf(normalized, "%d,%d", &w, &h); err != nil || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("invalid window size %q, expected W,H", s)
	}
	return w, h, nil
}

// IsAttach indica se o servidor deve conectar num Chrome existente em vez de lançar um.
func (c Config) IsAttach() bool { return c.Attach != "" || c.AttachPort != 0 }

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// ReadFile é o leitor real usado em produção.
var ReadFile = os.ReadFile
