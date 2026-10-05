package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/config"
)

// Cofre de senhas (docs/RETOQUES.md, decisão 14.2). O Chrome cifra cookies e senhas do perfil com uma
// chave: no macOS ela fica no Keychain, a menos que o navegador abra com --use-mock-keychain (chave
// fixa, que o rod põe por padrão). No Windows (DPAPI) e no Linux (libsecret, KWallet ou "basic") a
// bandeira não muda nada. As duas formas de abrir o perfil (a sessão da IA e o vitral open) precisam
// sempre da mesma chave, ou os logins de uma não abrem na outra: por isso a decisão é por perfil
// (UseMockKeychain) e os perfis antigos do macOS migram uma vez.

// storeMarker marca o perfil que já usa o cofre do sistema.
const storeMarker = ".vitral-password-store"

// goos é o sistema (trocado nos testes).
var goos = runtime.GOOS

// systemStore tira a chave fixa do lançamento (trocado nos testes, que nunca tocam o Keychain).
var systemStore = func(l *launcher.Launcher) *launcher.Launcher { return l.Delete("use-mock-keychain") }

// UseMockKeychain decide a chave de um lançamento do perfil e faz a migração quando é a hora. Devolve
// true quando o lançamento deve usar a chave fixa (--password-store mock, perfil temporário, ou
// migração que falhou: tenta de novo no próximo lançamento, sem perder os logins).
func UseMockKeychain(cfg config.Config, bin, profile string, log func(msg string, args ...any)) bool {
	if cfg.PasswordStore == "mock" {
		return true
	}
	if goos != "darwin" {
		return false
	}
	if _, err := os.Stat(filepath.Join(profile, storeMarker)); err == nil {
		return false
	}
	if hasCookies(profile) {
		if err := migrateToSystem(bin, profile); err != nil {
			if log != nil {
				log("password store migration failed; this launch keeps the old key and tries again next time", "profile", profile, "err", err)
			}
			return true
		}
	}
	if err := os.WriteFile(filepath.Join(profile, storeMarker), []byte("system\n"), 0o600); err != nil {
		if log != nil {
			log("password store marker", "err", err)
		}
		return true
	}
	return false
}

// hasCookies diz se o perfil já tem cookies gravados (Chrome novo: Default/Network/Cookies).
func hasCookies(profile string) bool {
	for _, p := range []string{filepath.Join("Default", "Network", "Cookies"), filepath.Join("Default", "Cookies")} {
		if _, err := os.Stat(filepath.Join(profile, p)); err == nil {
			return true
		}
	}
	return false
}

// migrateToSystem lê os cookies com a chave fixa e os regrava com a do sistema, em dois lançamentos
// sem janela. Os cookies ficam só em memória. Senhas salvas no gerenciador do Chrome não migram (no
// Vitral, quem digita as senhas é a pessoa).
func migrateToSystem(bin, profile string) error {
	var cookies []*proto.NetworkCookie
	err := withHeadless(bin, profile, func(l *launcher.Launcher) *launcher.Launcher { return l }, func(b *rod.Browser) error {
		res, err := proto.StorageGetCookies{}.Call(b)
		if err != nil {
			return err
		}
		for _, c := range res.Cookies {
			if !c.Session { // cookies de sessão não ficam no disco
				cookies = append(cookies, c)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("read cookies with the old key: %w", err)
	}
	if len(cookies) == 0 {
		return nil
	}
	err = withHeadless(bin, profile, systemStore, func(b *rod.Browser) error {
		return proto.StorageSetCookies{Cookies: proto.CookiesToParams(cookies)}.Call(b)
	})
	if err != nil {
		return fmt.Errorf("write cookies with the system key: %w", err)
	}
	return nil
}

// withHeadless abre o perfil sem janela, roda fn e fecha esperando o perfil ficar livre.
func withHeadless(bin, profile string, flags func(*launcher.Launcher) *launcher.Launcher, fn func(*rod.Browser) error) error {
	l := flags(launcher.New().Bin(bin).Headless(true).UserDataDir(profile).Set("remote-debugging-address", "127.0.0.1"))
	u, err := l.Launch()
	if err != nil {
		return err
	}
	defer func() {
		l.Kill()
		waitProfileFree(profile, 10*time.Second)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b := rod.New().ControlURL(u).Context(ctx)
	if err := b.Connect(); err != nil {
		return err
	}
	ferr := fn(b)
	// Fechar pelo CDP grava os cookies no disco antes de o processo sair.
	cerr := b.Close()
	return errors.Join(ferr, cerr)
}

// waitProfileFree espera o Chrome soltar o perfil (o SingletonLock some quando ele sai).
func waitProfileFree(profile string, max time.Duration) {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(filepath.Join(profile, "SingletonLock")); os.IsNotExist(err) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
