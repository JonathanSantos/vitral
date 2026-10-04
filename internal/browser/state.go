package browser

import (
	"fmt"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/storagestate"
)

// Estado de sessão (pedido "próximos passos", 4.2). O localStorage de uma origem que não está aberta
// não é alcançável pelo DOMStorage do CDP ("Frame not found"; medido); por isso a leitura e a gravação
// usam uma aba temporária na própria origem, com a requisição interceptada devolvendo uma página
// vazia: sem rede, sem script da página, ~90 ms por origem.

// maxCookieOrigins limita as origens tiradas dos cookies quando o perfil não registrou origens.
const maxCookieOrigins = 30

// originTimeout é o limite de cada aba temporária.
const originTimeout = 10 * time.Second

// visited são as origens registradas no perfil em uso.
func (m *Manager) visited() storagestate.Visited { return storagestate.Visited{Dir: m.profileDir()} }

// trackOrigins registra a origem de cada navegação do frame principal (só a origem, nada da página).
func (m *Manager) trackOrigins(p *rod.Page) {
	v := m.visited()
	go p.EachEvent(func(e *proto.PageFrameNavigated) {
		if e.Frame != nil && e.Frame.ParentID == "" {
			_, _ = v.Add(storagestate.OriginOf(e.Frame.URL))
		}
	})()
}

// SaveState salva cookies e o localStorage das origens visitadas em sessionsDir/<name>.json (0600).
// Devolve só o resumo: o conteúdo nunca vai para a IA.
func (m *Manager) SaveState(name string) (storagestate.Summary, error) {
	if !config.ValidName(name) {
		return storagestate.Summary{}, fmt.Errorf("invalid session name %q: use lowercase letters, digits, - and _", name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureBrowser(); err != nil {
		return storagestate.Summary{}, err
	}
	res, err := proto.StorageGetCookies{}.Call(m.browser)
	if err != nil {
		return storagestate.Summary{}, fmt.Errorf("read cookies: %w", err)
	}
	st := storagestate.State{Name: name, Profile: m.cfg.Profile, Created: time.Now().UTC().Truncate(time.Second)}
	for _, c := range res.Cookies {
		st.Cookies = append(st.Cookies, storagestate.Cookie{
			Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, HTTPOnly: c.HTTPOnly, Secure: c.Secure,
			SameSite: string(c.SameSite), Expires: expiresOf(c),
		})
	}
	origins := m.visited().List()
	if len(origins) == 0 {
		origins = storagestate.Merge(storagestate.CookieOrigins(st.Cookies))
		origins = origins[:min(len(origins), maxCookieOrigins)]
	}
	for _, o := range origins {
		var items map[string]string
		err := m.withOriginTab(o, func(p *rod.Page) error {
			r, err := p.Eval(`() => Object.assign({}, localStorage)`)
			if err != nil {
				return err
			}
			return r.Value.Unmarshal(&items)
		})
		if err != nil {
			m.log.Debug("read localStorage", "origin", o, "err", err)
			continue
		}
		if len(items) > 0 {
			st.Origins = append(st.Origins, storagestate.Origin{Origin: o, LocalStorage: items})
		}
	}
	path, err := storagestate.Store{Dir: m.cfg.SessionsDir}.Save(st)
	if err != nil {
		return storagestate.Summary{}, err
	}
	return st.Summary(path), nil
}

func expiresOf(c *proto.NetworkCookie) float64 {
	if c.Session {
		return 0
	}
	return float64(c.Expires)
}

// injectState põe o estado salvo no navegador recém-aberto, antes da primeira navegação: o
// localStorage já existe quando o primeiro script da página roda.
func (m *Manager) injectState(st storagestate.State) error {
	var params []*proto.NetworkCookieParam
	for _, c := range st.Cookies {
		p := &proto.NetworkCookieParam{
			Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path,
			HTTPOnly: c.HTTPOnly, Secure: c.Secure, SameSite: proto.NetworkCookieSameSite(c.SameSite),
		}
		if c.Expires > 0 {
			p.Expires = proto.TimeSinceEpoch(c.Expires)
		}
		params = append(params, p)
	}
	if len(params) > 0 {
		if err := (proto.StorageSetCookies{Cookies: params}).Call(m.browser); err != nil {
			return fmt.Errorf("restore cookies: %w", err)
		}
	}
	for _, o := range st.Origins {
		err := m.withOriginTab(o.Origin, func(p *rod.Page) error {
			_, err := p.Eval(`(items) => { for (const [k, v] of Object.entries(items)) localStorage.setItem(k, v); }`, o.LocalStorage)
			return err
		})
		if err != nil {
			return fmt.Errorf("restore localStorage of %s: %w", o.Origin, err)
		}
		_, _ = m.visited().Add(o.Origin)
	}
	return nil
}

// withOriginTab abre uma aba temporária na origem com a requisição interceptada (página vazia, sem
// rede) e roda fn nela.
func (m *Manager) withOriginTab(origin string, fn func(p *rod.Page) error) error {
	tab, err := m.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return err
	}
	defer func() { _ = tab.Close() }()
	tab = tab.Timeout(originTimeout)
	router := tab.HijackRequests()
	if err := router.Add(origin+"/*", "", func(h *rod.Hijack) {
		h.Response.SetHeader("Content-Type", "text/html; charset=utf-8")
		h.Response.SetBody("<!doctype html><title></title>")
	}); err != nil {
		return err
	}
	go router.Run()
	defer func() { _ = router.Stop() }()
	if err := tab.Navigate(origin + "/__vitral_storage__"); err != nil {
		return err
	}
	if err := tab.WaitLoad(); err != nil {
		return err
	}
	return fn(tab)
}

// profileDir é o perfil em uso (o temporário, com --session).
func (m *Manager) profileDir() string {
	if m.tempProfile != "" {
		return m.tempProfile
	}
	return m.cfg.ProfileDir
}

// sessionProfile prepara o perfil temporário do --session (apagado no Close).
func (m *Manager) sessionProfile() (string, error) {
	if m.cfg.Session == "" {
		return m.cfg.ProfileDir, nil
	}
	if _, err := (storagestate.Store{Dir: m.cfg.SessionsDir}).Load(m.cfg.Session); err != nil {
		return "", fmt.Errorf("%w. Save it first with session_save (or: vitral session save %s)", err, m.cfg.Session)
	}
	dir, err := os.MkdirTemp("", config.Name+"-session-*")
	if err != nil {
		return "", err
	}
	m.tempProfile = dir
	return dir, nil
}
