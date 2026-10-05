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
	"regexp"
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
	// Profile é o perfil nomeado (~/.vitral/profiles/<nome>): contas diferentes, homologação e
	// produção. ProfileDir, se informado, vence.
	Profile    string `yaml:"profile"`
	ProfileDir string `yaml:"profileDir"`
	// Session inicia com um perfil temporário e o estado salvo por session_save (cookies e
	// localStorage), para rodar sem janela ou com o perfil aberto em outro Vitral.
	Session     string `yaml:"session"`
	SessionsDir string `yaml:"sessionsDir"`
	// NoMap desliga o mapa de telas; MapsDir guarda um mapa por host.
	NoMap   bool   `yaml:"noMap"`
	MapsDir string `yaml:"mapsDir"`
	// NoRetouches desliga os Retoques (docs/RETOQUES.md); RetouchesDir guarda um retoque por pasta.
	NoRetouches  bool   `yaml:"noRetouches"`
	RetouchesDir string `yaml:"retouchesDir"`
	// PasswordStore é onde o Chrome guarda a chave dos cookies e senhas do perfil (decisão 14.2 de
	// docs/RETOQUES.md): "system" (Keychain, DPAPI, libsecret) ou "mock" (chave fixa: só testes e
	// perfis temporários). Igual na sessão da IA e no vitral open, sempre.
	PasswordStore string `yaml:"passwordStore"`
	// BrowserPath força um executável específico (--browser-path / VITRAL_BROWSER_PATH).
	BrowserPath string `yaml:"browserPath"`
	// AllowSystemBrowser permite usar o Chrome/Edge do sistema como último recurso (sempre com o perfil do Vitral).
	AllowSystemBrowser bool `yaml:"allowSystemBrowser"`
	// BrowserMirror é a base URL alternativa para baixar o Chrome for Testing (mesmo layout de paths).
	BrowserMirror string `yaml:"browserMirror"`
	// BrowsersDir guarda os Chrome for Testing baixados pelo Vitral.
	BrowsersDir string `yaml:"browsersDir"`
	WindowSize  string `yaml:"windowSize"`

	AllowEval    bool     `yaml:"allowEval"`
	AllowedHosts []string `yaml:"allowedHosts"`
	DisableMock  bool     `yaml:"disableMock"`
	MaskHeaders  []string `yaml:"maskHeaders"`

	NavTimeout       time.Duration `yaml:"navTimeout"`
	StabilityTimeout time.Duration `yaml:"stabilityTimeout"`

	NetworkBuffer   int  `yaml:"networkBuffer"`
	PreserveNetwork bool `yaml:"preserveNetwork"`
	ConsoleBuffer   int  `yaml:"consoleBuffer"`

	// HighlightMs é o tempo do destaque antes de cada ação; -1 = automático (350 ms, 0 em headless).
	HighlightMs int  `yaml:"highlightMs"`
	NoOverlay   bool `yaml:"noOverlay"`

	// Cursor visual da IA (docs/CURSOR.md): on (só com janela visível), off ou force (liga mesmo
	// sem janela visível, para testes e gravação de demos).
	Cursor          string `yaml:"cursor"`
	CursorSpeed     string `yaml:"cursorSpeed"`     // slow, normal, fast ou instant
	FlowCursorSpeed string `yaml:"flowCursorSpeed"` // velocidade durante flow_run
	CursorSeed      int64  `yaml:"cursorSeed"`      // 0 = aleatória por sessão
	TypePacing      string `yaml:"typePacing"`      // on (só com janela visível) ou off

	// Lang fixa o idioma do navegador (ex.: pt-BR); vazio = idioma do sistema.
	Lang string `yaml:"lang"`
	// DebugInfoFile recebe a URL do CDP e o PID do navegador lançado (testes e diagnóstico).
	DebugInfoFile string `yaml:"debugInfoFile"`

	FlowsDir string `yaml:"flowsDir"`
	LogLevel string `yaml:"logLevel"`
}

// Defaults devolve a configuração padrão. homeDir vem de fora para manter a função pura.
func Defaults(homeDir string) Config {
	return Config{
		Profile:          DefaultProfile,
		SessionsDir:      filepath.Join(homeDir, "."+Name, "sessions"),
		MapsDir:          filepath.Join(homeDir, "."+Name, "maps"),
		RetouchesDir:     filepath.Join(homeDir, "."+Name, "retoques"),
		PasswordStore:    "system",
		BrowsersDir:      filepath.Join(homeDir, "."+Name, "browsers"),
		WindowSize:       "1280,800",
		NavTimeout:       15 * time.Second,
		StabilityTimeout: 10 * time.Second,
		NetworkBuffer:    500,
		ConsoleBuffer:    200,
		HighlightMs:      -1,
		Cursor:           "on",
		CursorSpeed:      "normal",
		FlowCursorSpeed:  "fast",
		TypePacing:       "on",
		FlowsDir:         filepath.Join("."+Name, "flows"),
		LogLevel:         "info",
	}
}

// DefaultConfigFile é o arquivo lido automaticamente quando existe no diretório de trabalho.
const DefaultConfigFile = Name + ".yaml"

// Load aplica, nesta ordem: defaults, arquivo yaml, variáveis de ambiente e flags (flags vencem).
func Load(args []string, homeDir string, readFile func(string) ([]byte, error), getenv func(string) string) (Config, error) {
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

	applyEnv(&cfg, getenv)
	if err := parseFlags(&cfg, args); err != nil {
		return cfg, err
	}
	if cfg.ProfileDir == "" {
		cfg.ProfileDir = ProfilePath(homeDir, cfg.Profile)
	}
	cfg.ProfileDir = expandHome(cfg.ProfileDir, homeDir)
	cfg.SessionsDir = expandHome(cfg.SessionsDir, homeDir)
	cfg.MapsDir = expandHome(cfg.MapsDir, homeDir)
	cfg.RetouchesDir = expandHome(cfg.RetouchesDir, homeDir)
	cfg.BrowsersDir = expandHome(cfg.BrowsersDir, homeDir)
	cfg.BrowserPath = expandHome(cfg.BrowserPath, homeDir)
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

// Variáveis de ambiente úteis em redes corporativas e instalações gerenciadas.
const (
	EnvBrowserPath   = "VITRAL_BROWSER_PATH"
	EnvBrowserMirror = "VITRAL_BROWSER_MIRROR"
)

func applyEnv(cfg *Config, getenv func(string) string) {
	if getenv == nil {
		return
	}
	if v := getenv(EnvBrowserPath); v != "" {
		cfg.BrowserPath = v
	}
	if v := getenv(EnvBrowserMirror); v != "" {
		cfg.BrowserMirror = v
	}
}

func parseFlags(cfg *Config, args []string) error {
	fset := flag.NewFlagSet(Name, flag.ContinueOnError)
	fset.SetOutput(io.Discard)

	fset.String("config", DefaultConfigFile, "path to the yaml config file")
	fset.BoolVar(&cfg.Headless, "headless", cfg.Headless, "run the browser without a window")
	fset.StringVar(&cfg.Attach, "attach", cfg.Attach, "attach to a running Chrome by websocket URL (ws://...)")
	fset.IntVar(&cfg.AttachPort, "attach-port", cfg.AttachPort, "attach to a running Chrome by remote debugging port")
	fset.StringVar(&cfg.Profile, "profile", cfg.Profile, "named browser profile in ~/.vitral/profiles (e.g. hml, prod)")
	fset.StringVar(&cfg.ProfileDir, "profile-dir", cfg.ProfileDir, "dedicated browser profile directory (overrides --profile)")
	fset.StringVar(&cfg.Session, "session", cfg.Session, "start with a temporary profile and the login state saved by session_save")
	fset.StringVar(&cfg.SessionsDir, "sessions-dir", cfg.SessionsDir, "where saved sessions are kept")
	fset.BoolVar(&cfg.NoMap, "no-map", cfg.NoMap, "do not learn the screen map")
	fset.StringVar(&cfg.MapsDir, "maps-dir", cfg.MapsDir, "where screen maps are kept (one per host)")
	fset.BoolVar(&cfg.NoRetouches, "no-retouches", cfg.NoRetouches, "do not load or create retouches (CSS changes kept for a site)")
	fset.StringVar(&cfg.RetouchesDir, "retouches-dir", cfg.RetouchesDir, "where retouches are kept (one folder each)")
	fset.StringVar(&cfg.PasswordStore, "password-store", cfg.PasswordStore, "where the browser keeps the key of cookies and passwords: system (OS keychain) or mock (fixed key, for tests)")
	fset.StringVar(&cfg.BrowserPath, "browser-path", cfg.BrowserPath, "use this Chrome/Chromium executable (env "+EnvBrowserPath+")")
	fset.BoolVar(&cfg.AllowSystemBrowser, "allow-system-browser", cfg.AllowSystemBrowser, "allow the installed Chrome/Edge as a last resort (always with the Vitral profile)")
	fset.StringVar(&cfg.BrowserMirror, "browser-mirror", cfg.BrowserMirror, "base URL of a Chrome for Testing mirror (env "+EnvBrowserMirror+")")
	fset.StringVar(&cfg.BrowsersDir, "browsers-dir", cfg.BrowsersDir, "where Vitral keeps its Chrome for Testing")
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
	fset.IntVar(&cfg.HighlightMs, "highlight-ms", cfg.HighlightMs, "how long the target blinks before each action (0 disables; default 350, off in headless)")
	fset.BoolVar(&cfg.NoOverlay, "no-overlay", cfg.NoOverlay, "hide the highlight and status badge (the pause button stays)")
	fset.StringVar(&cfg.Cursor, "cursor", cfg.Cursor, "AI cursor: on (visible window only), off, or force (on even without a visible window: tests and demo recordings)")
	fset.StringVar(&cfg.CursorSpeed, "cursor-speed", cfg.CursorSpeed, "AI cursor speed: slow, normal, fast or instant")
	fset.StringVar(&cfg.FlowCursorSpeed, "flow-cursor-speed", cfg.FlowCursorSpeed, "AI cursor speed during flow_run")
	fset.Int64Var(&cfg.CursorSeed, "cursor-seed", cfg.CursorSeed, "fixed seed for the AI cursor movement (0 = random per session)")
	fset.StringVar(&cfg.TypePacing, "type-pacing", cfg.TypePacing, "type visibly key by key with a visible window: on or off")
	fset.StringVar(&cfg.Lang, "lang", cfg.Lang, "browser language, e.g. pt-BR (default: system)")
	fset.StringVar(&cfg.DebugInfoFile, "debug-info", cfg.DebugInfoFile, "write the browser CDP URL and PID to this file (tests and diagnostics)")
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
	if !ValidName(cfg.Profile) {
		return fmt.Errorf("--profile must use lowercase letters, digits, - and _ (got %q)", cfg.Profile)
	}
	if cfg.Session != "" && !ValidName(cfg.Session) {
		return fmt.Errorf("--session must use lowercase letters, digits, - and _ (got %q)", cfg.Session)
	}
	if cfg.Session != "" && (cfg.Attach != "" || cfg.AttachPort != 0) {
		return errors.New("--session starts its own browser: it cannot be used with --attach")
	}
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
	if !oneOf(cfg.PasswordStore, "system", "mock") {
		return fmt.Errorf("--password-store must be system or mock, got %q", cfg.PasswordStore)
	}
	if !oneOf(cfg.Cursor, "on", "off", "force") {
		return fmt.Errorf("--cursor must be on, off or force, got %q", cfg.Cursor)
	}
	for name, v := range map[string]string{"--cursor-speed": cfg.CursorSpeed, "--flow-cursor-speed": cfg.FlowCursorSpeed} {
		if !oneOf(v, "slow", "normal", "fast", "instant") {
			return fmt.Errorf("%s must be slow, normal, fast or instant, got %q", name, v)
		}
	}
	if !oneOf(cfg.TypePacing, "on", "off") {
		return fmt.Errorf("--type-pacing must be on or off, got %q", cfg.TypePacing)
	}
	return nil
}

func oneOf(v string, options ...string) bool {
	for _, o := range options {
		if v == o {
			return true
		}
	}
	return false
}

// CursorActive diz se o cursor visual aparece: com janela visível (ou force) e com o overlay.
func (c Config) CursorActive() bool {
	if c.NoOverlay || c.Cursor == "off" {
		return false
	}
	return c.Cursor == "force" || !c.Headless
}

// TypePacingActive diz se a digitação sai tecla a tecla, para a pessoa ver o texto aparecendo.
func (c Config) TypePacingActive() bool {
	return c.TypePacing == "on" && (c.Cursor == "force" || !c.Headless)
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

// Highlight é o tempo efetivo do destaque: em headless ninguém vê, então só liga se pedido.
func (c Config) Highlight() time.Duration {
	if c.NoOverlay {
		return 0
	}
	if c.HighlightMs < 0 {
		if c.Headless {
			return 0
		}
		return 350 * time.Millisecond
	}
	return time.Duration(c.HighlightMs) * time.Millisecond
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

// DefaultProfile é o perfil usado sem --profile.
const DefaultProfile = "default"

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// ValidName aceita nomes de perfil e de sessão que viram nomes de pasta e de arquivo seguros.
func ValidName(name string) bool { return validName.MatchString(name) }

// ProfilePath é a pasta do perfil nomeado.
func ProfilePath(homeDir, profile string) string {
	return filepath.Join(homeDir, "."+Name, "profiles", profile)
}

// MigrateProfile move o perfil antigo (~/.vitral/profile) para ~/.vitral/profiles/default, só se o
// destino ainda não existe. Idempotente; moved diz se mudou algo.
func MigrateProfile(homeDir string) (moved bool, err error) {
	old := filepath.Join(homeDir, "."+Name, "profile")
	dst := ProfilePath(homeDir, DefaultProfile)
	if _, err := os.Stat(old); err != nil {
		return false, nil
	}
	if _, err := os.Stat(dst); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return false, err
	}
	if err := os.Rename(old, dst); err != nil {
		return false, err
	}
	return true, nil
}
