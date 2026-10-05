package browser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JonathanSantos/vitral/internal/config"
)

// A regra do cofre de senhas, sem navegador: mock, outro sistema, perfil já migrado, perfil sem
// cookies (só grava a marca) e migração que falha (mantém a chave fixa e tenta de novo depois).
func TestUseMockKeychain(t *testing.T) {
	defer func(orig string) { goos = orig }(goos)
	sys := config.Config{PasswordStore: "system"}
	var logged []string
	log := func(msg string, _ ...any) { logged = append(logged, msg) }

	if !UseMockKeychain(config.Config{PasswordStore: "mock"}, "", t.TempDir(), log) {
		t.Fatal("mock store must use the fixed key")
	}
	for _, osName := range []string{"linux", "windows"} {
		goos = osName
		profile := t.TempDir()
		if UseMockKeychain(sys, "", profile, log) {
			t.Fatalf("%s: the flag changes nothing there, the system store is the default", osName)
		}
		if _, err := os.Stat(filepath.Join(profile, storeMarker)); !os.IsNotExist(err) {
			t.Fatalf("%s: no marker needed: %v", osName, err)
		}
	}

	goos = "darwin"
	fresh := t.TempDir()
	if UseMockKeychain(sys, "/no/browser", fresh, log) {
		t.Fatal("a profile without cookies goes straight to the system store")
	}
	if data, err := os.ReadFile(filepath.Join(fresh, storeMarker)); err != nil || string(data) != "system\n" {
		t.Fatalf("marker: %q %v", data, err)
	}
	if UseMockKeychain(sys, "/no/browser", fresh, log) {
		t.Fatal("a migrated profile stays on the system store")
	}

	old := t.TempDir()
	if err := os.MkdirAll(filepath.Join(old, "Default", "Network"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "Default", "Network", "Cookies"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	logged = nil
	if !UseMockKeychain(sys, "/no/browser", old, log) {
		t.Fatal("a failed migration keeps the fixed key for this launch")
	}
	if _, err := os.Stat(filepath.Join(old, storeMarker)); !os.IsNotExist(err) {
		t.Fatalf("no marker after a failed migration: %v", err)
	}
	if len(logged) != 1 {
		t.Fatalf("the failure is logged once: %v", logged)
	}
	if !UseMockKeychain(sys, "/no/browser", old, nil) {
		t.Fatal("still the fixed key without a logger")
	}

	// Cookies no formato antigo (Default/Cookies) também contam.
	older := t.TempDir()
	if err := os.MkdirAll(filepath.Join(older, "Default"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(older, "Default", "Cookies"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasCookies(older) || hasCookies(t.TempDir()) {
		t.Fatal("hasCookies")
	}

	// Marca que não dá para gravar: fica na chave fixa.
	ro := filepath.Join(t.TempDir(), "nao-existe", "perfil")
	if !UseMockKeychain(sys, "", ro, log) {
		t.Fatal("without the marker the launch keeps the fixed key")
	}
}
