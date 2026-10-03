// Package browser lança ou conecta no Chromium e mantém a página ativa.
package browser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"

	"github.com/JonathanSantos/vitral/internal/config"
)

// PageHook prepara cada página nova (scripts, listeners de rede/console, mocks).
type PageHook func(*rod.Page) error

// Manager guarda o browser e a única página ativa da v1.
// Para múltiplas abas, a página viraria um mapa por id, cada uma com sua sessão.
type Manager struct {
	cfg   config.Config
	log   *slog.Logger
	hooks []PageHook

	// download recebe o progresso do download do Chromium (a CLI traduz para PT-BR).
	download utils.Logger

	mu       sync.Mutex
	launched *launcher.Launcher
	browser  *rod.Browser
	page     *rod.Page
	cancel   context.CancelFunc
}

// New cria o gerenciador sem subir nada: o browser só abre na primeira ferramenta.
func New(cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{cfg: cfg, log: logger}
}

// SetDownloadLogger define quem mostra o progresso do download do Chromium (padrão: stderr cru).
func (m *Manager) SetDownloadLogger(l utils.Logger) { m.download = l }

// OnNewPage registra um preparo executado em toda página nova. Chamar antes do primeiro uso.
func (m *Manager) OnNewPage(h PageHook) {
	m.hooks = append(m.hooks, h)
}

// Page devolve a página ativa, subindo o browser ou reabrindo a aba quando necessário.
func (m *Manager) Page() (*rod.Page, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureBrowser(); err != nil {
		return nil, err
	}
	if m.page != nil && m.pageAlive() {
		return m.page, nil
	}
	return m.openPage()
}

// Current devolve a página ativa sem criar nada (nil se ainda não existe).
func (m *Manager) Current() *rod.Page {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.page
}

func (m *Manager) ensureBrowser() error {
	if m.browser != nil {
		if _, err := (proto.BrowserGetVersion{}).Call(m.browser); err == nil {
			return nil
		}
		m.log.Warn("browser connection lost, starting again")
		m.reset()
	}

	controlURL, err := m.controlURL()
	if err != nil {
		return err
	}
	b := rod.New().ControlURL(controlURL).NoDefaultDevice().Logger(log.New(os.Stderr, "[rod] ", log.LstdFlags))
	if err := b.Connect(); err != nil {
		m.killLaunched()
		return fmt.Errorf("connect to browser at %s: %w", controlURL, err)
	}
	m.browser = b
	return nil
}

func (m *Manager) controlURL() (string, error) {
	if m.cfg.Attach != "" {
		u, err := launcher.ResolveURL(m.cfg.Attach)
		if err != nil {
			return "", fmt.Errorf("attach to %s: %w. Start Chrome with --remote-debugging-port and a separate --user-data-dir", m.cfg.Attach, err)
		}
		return u, nil
	}
	if m.cfg.AttachPort != 0 {
		u, err := launcher.ResolveURL("127.0.0.1:" + strconv.Itoa(m.cfg.AttachPort))
		if err != nil {
			return "", fmt.Errorf("attach to port %d: %w. Start Chrome with --remote-debugging-port=%d and a separate --user-data-dir", m.cfg.AttachPort, err, m.cfg.AttachPort)
		}
		return u, nil
	}
	return m.launch()
}

func (m *Manager) launch() (string, error) {
	bin, err := m.browserBin()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(m.cfg.ProfileDir, 0o700); err != nil {
		return "", fmt.Errorf("create profile dir: %w", err)
	}
	w, h, _ := config.ParseWindowSize(m.cfg.WindowSize)

	l := launcher.New().
		Bin(bin).
		Headless(m.cfg.Headless).
		UserDataDir(m.cfg.ProfileDir).
		Set("window-size", strconv.Itoa(w), strconv.Itoa(h)).
		Set("remote-debugging-address", "127.0.0.1").
		Logger(m.chromeLogWriter())
	m.log.Info("launching browser", "bin", bin, "headless", m.cfg.Headless, "profile", m.cfg.ProfileDir)
	u, err := l.Launch()
	if err != nil {
		return "", fmt.Errorf("launch browser: %w. If another %s is using the profile %s, close it or use --profile-dir", err, config.Name, m.cfg.ProfileDir)
	}
	m.launched = l
	return u, nil
}

// browserBin prefere o binário configurado, depois o Chrome instalado e por fim baixa o Chromium.
func (m *Manager) browserBin() (string, error) {
	if m.cfg.BrowserBin != "" {
		return m.cfg.BrowserBin, nil
	}
	if bin, ok := launcher.LookPath(); ok {
		return bin, nil
	}
	m.log.Info("no Chrome found, downloading Chromium (first run only)")
	dl := launcher.NewBrowser()
	// O logger padrão do rod escreve em stdout, que é reservado ao protocolo MCP.
	dl.Logger = log.New(os.Stderr, "[download] ", log.LstdFlags)
	if m.download != nil {
		dl.Logger = m.download
	}
	bin, err := dl.Get()
	if err != nil {
		return "", fmt.Errorf("download Chromium: %w", err)
	}
	return bin, nil
}

func (m *Manager) chromeLogWriter() io.Writer {
	if m.log.Enabled(context.Background(), slog.LevelDebug) {
		return os.Stderr
	}
	return io.Discard
}

func (m *Manager) pageAlive() bool {
	info, err := (proto.TargetGetTargetInfo{TargetID: m.page.TargetID}).Call(m.browser)
	return err == nil && info.TargetInfo != nil
}

func (m *Manager) openPage() (*rod.Page, error) {
	if m.cancel != nil {
		m.cancel()
	}
	p, err := m.pickPage()
	if err != nil {
		return nil, fmt.Errorf("open tab: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p = p.Context(ctx)
	for _, hook := range m.hooks {
		if err := hook(p); err != nil {
			cancel()
			return nil, fmt.Errorf("prepare tab: %w", err)
		}
	}
	if _, err := p.Activate(); err != nil {
		m.log.Debug("activate tab", "err", err)
	}
	m.page, m.cancel = p, cancel
	return p, nil
}

// pickPage reaproveita uma aba vazia do browser que nós lançamos; no attach sempre cria uma aba nova.
func (m *Manager) pickPage() (*rod.Page, error) {
	if m.launched != nil {
		pages, err := m.browser.Pages()
		if err == nil {
			for _, p := range pages {
				info, err := p.Info()
				if err == nil && isBlank(info.URL) {
					return p, nil
				}
			}
		}
	}
	return m.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
}

func isBlank(u string) bool {
	return u == "about:blank" || u == "" || strings.HasPrefix(u, "chrome://newtab") || strings.HasPrefix(u, "chrome://new-tab-page")
}

// Close fecha o browser que lançamos. No attach, só solta a conexão.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	if m.launched == nil {
		return
	}
	if m.browser != nil {
		if err := m.browser.Close(); err != nil && !errors.Is(err, context.Canceled) {
			m.log.Debug("close browser", "err", err)
		}
	}
	m.killLaunched()
}

func (m *Manager) reset() {
	if m.cancel != nil {
		m.cancel()
	}
	m.killLaunched()
	m.browser, m.page, m.cancel = nil, nil, nil
}

func (m *Manager) killLaunched() {
	if m.launched == nil {
		return
	}
	m.launched.Kill()
	m.launched = nil
}
