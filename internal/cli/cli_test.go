package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/config"
)

// fakeEnv monta um ambiente isolado: home e projeto em pastas temporárias e um "binário" falso.
type fakeEnv struct {
	Env
	out      *bytes.Buffer
	versions map[string]string // caminho do binário -> versão que ele informa
	gitFiles string
}

func newFakeEnv(t *testing.T, goos string) *fakeEnv {
	t.Helper()
	root := t.TempDir()
	home, cwd := filepath.Join(root, "home"), filepath.Join(root, "proj")
	for _, d := range []string{home, cwd} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(root, "download", "vitral")
	writeFile(t, exe, "binary v-new")
	f := &fakeEnv{out: &bytes.Buffer{}, versions: map[string]string{}}
	f.Env = Env{
		Home: home, Cwd: cwd, GOOS: goos, Executable: exe,
		Getenv: func(string) string { return "" },
		Stdin:  strings.NewReader(""), Stdout: f.out, Stderr: f.out,
		Run: func(dir, name string, args ...string) (string, error) {
			if name == "git" {
				return f.gitFiles, nil
			}
			// O binário instalado informa a versão conforme o conteúdo do arquivo.
			data, err := os.ReadFile(name)
			if err != nil {
				return "", err
			}
			if v, ok := f.versions[string(data)]; ok {
				return config.Name + " " + v, nil
			}
			return "", errors.New("unknown binary")
		},
	}
	f.versions["binary v-new"] = config.Version
	return f
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMainDispatch(t *testing.T) {
	load := func(args []string) (config.Config, error) {
		if len(args) > 0 {
			return config.Config{}, errors.New("bad flag " + args[0])
		}
		return config.Config{}, nil
	}
	tests := []struct {
		name        string
		args        []string
		interactive bool
		handled     bool
		code        int
		out         string
	}{
		{"terminal without args shows help", nil, true, true, 0, "vitral setup"},
		{"pipe without args runs the MCP server", nil, false, false, 0, ""},
		{"flags run the MCP server", []string{"--headless"}, true, false, 0, ""},
		{"version", []string{"version"}, false, true, 0, config.Name + " " + config.Version},
		{"--version", []string{"--version"}, false, true, 0, config.Name + " "},
		{"unknown command", []string{"instalar"}, true, true, 1, "comando desconhecido"},
		{"bad setup flag", []string{"setup", "--force"}, true, true, 1, "opção desconhecida"},
		{"disable with nothing configured", []string{"disable"}, true, true, 0, "nada mudou"},
		{"enable with nothing disabled", []string{"enable", "--workspace"}, true, true, 0, "nada mudou"},
		{"bad disable flag", []string{"disable", "--tudo"}, true, true, 1, `"--tudo" para disable`},
		{"status", []string{"status"}, true, true, 0, "Configuração:"},
		{"status with bad flag", []string{"status", "--nope"}, true, true, 1, "flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeEnv(t, "linux")
			f.Interactive = tt.interactive
			handled, code := Main(tt.args, f.Env, load)
			if handled != tt.handled || code != tt.code || !strings.Contains(f.out.String(), tt.out) {
				t.Fatalf("handled=%v code=%d out=%q", handled, code, f.out.String())
			}
		})
	}
}

func TestIsTerminalWithPipeAndDevNull(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if IsTerminal(r) {
		t.Fatal("a pipe (how VS Code starts Vitral) is not a terminal")
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if IsTerminal(null) {
		t.Fatal("/dev/null is not a terminal")
	}
}

func TestSetupFirstRun(t *testing.T) {
	f := newFakeEnv(t, "linux")
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(f.Home, ".vitral", "bin", "vitral")
	if readFile(t, installed) != "binary v-new" {
		t.Fatal("binary not installed")
	}
	// Formato portável: .mcp.json na raiz, chave mcpServers, todas as ferramentas.
	mcp := readFile(t, filepath.Join(f.Cwd, ".mcp.json"))
	entry, ok, err := ReadServer([]byte(mcp), KeyPortable, ServerName)
	if err != nil || !ok || entry.Command != installed || entry.Type != "stdio" || len(entry.Tools) != 1 || entry.Tools[0] != "*" {
		t.Fatalf(".mcp.json = %s (%v)", mcp, err)
	}
	if _, err := os.Stat(filepath.Join(f.Cwd, ".vscode", "mcp.json")); err == nil {
		t.Fatal("the deprecated .vscode/mcp.json must not be created")
	}
	prompts, _ := os.ReadDir(filepath.Join(f.Cwd, ".github", "prompts"))
	if len(prompts) != 5 {
		t.Fatalf("want 5 prompts, got %d", len(prompts))
	}
	out := f.out.String()
	for _, want := range []string{"Vitral instalado em", "5 novos", "Próximos passos", "MCP: List Servers"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(f.Cwd, ".mcp.json.bak")); err == nil {
		t.Error("no backup expected for a new file")
	}
}

func TestSetupMergesAndDoesNotOverwrite(t *testing.T) {
	f := newFakeEnv(t, "linux")
	original := `{
  // configuração da equipe
  "mcpServers": {
    "github": { "type": "http", "url": "https://api.githubcopilot.com/mcp/" },
  },
}`
	writeFile(t, filepath.Join(f.Cwd, ".mcp.json"), original)
	writeFile(t, filepath.Join(f.Cwd, ".github", "prompts", "vitral-descrever-tela.prompt.md"), "minha versão")

	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	mcp := readFile(t, filepath.Join(f.Cwd, ".mcp.json"))
	for _, want := range []string{"// configuração da equipe", `"github": { "type": "http"`, `"vitral"`} {
		if !strings.Contains(mcp, want) {
			t.Errorf("missing %q in merged .mcp.json:\n%s", want, mcp)
		}
	}
	if readFile(t, filepath.Join(f.Cwd, ".mcp.json.bak")) != original {
		t.Error("backup must hold the original file")
	}
	// Sem terminal e sem --yes, o prompt editado pela pessoa fica como está.
	if readFile(t, filepath.Join(f.Cwd, ".github", "prompts", "vitral-descrever-tela.prompt.md")) != "minha versão" {
		t.Fatal("existing prompt must not be overwritten without confirmation")
	}
	if !strings.Contains(f.out.String(), "4 novos, 0 atualizados, 0 iguais, 1 mantidos") {
		t.Fatalf("unexpected summary:\n%s", f.out.String())
	}

	// Com confirmação interativa ("s"), substitui.
	f.out.Reset()
	f.Interactive = true
	f.Stdin = strings.NewReader("s\n")
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(f.Cwd, ".github", "prompts", "vitral-descrever-tela.prompt.md")) == "minha versão" {
		t.Fatal("confirmed prompt should be replaced")
	}
	if !strings.Contains(f.out.String(), "0 novos, 1 atualizados, 4 iguais, 0 mantidos") {
		t.Fatalf("unexpected summary:\n%s", f.out.String())
	}
}

// O Vitral no formato antigo (.vscode/mcp.json) sai de lá, com confirmação: em dois lugares, o VS
// Code subiria dois servidores.
func TestSetupMigratesLegacyWorkspace(t *testing.T) {
	f := newFakeEnv(t, "linux")
	legacy := filepath.Join(f.Cwd, ".vscode", "mcp.json")
	original := `{
  "servers": {
    "github": { "type": "http", "url": "https://x" },
    "vitral": { "type": "stdio", "command": "/old/vitral", "args": [] }
  }
}`
	writeFile(t, legacy, original)

	// Sem terminal e sem --yes: mantém e avisa.
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.out.String(), "mantido: "+legacy) || readFile(t, legacy) != original {
		t.Fatalf("legacy entry must stay without confirmation:\n%s", f.out.String())
	}
	// Respondendo "s": tira só o Vitral, com cópia de segurança.
	f.out.Reset()
	f.Interactive, f.Stdin = true, strings.NewReader("s\n")
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, legacy)
	if strings.Contains(got, "vitral") || !strings.Contains(got, `"github"`) || readFile(t, legacy+".bak") != original {
		t.Fatalf("migration result:\n%s", got)
	}
	if !strings.Contains(f.out.String(), "tirei o Vitral do formato antigo em "+legacy) {
		t.Fatalf("%s", f.out.String())
	}
	// Desligado no formato antigo também sai (com --yes).
	writeFile(t, legacy, `{"servers": {}, "_disabled": {"vitral": {"command": "/old"}}}`)
	if err := Setup(f.Env, SetupOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, legacy), "vitral") {
		t.Fatalf("disabled legacy entry kept:\n%s", readFile(t, legacy))
	}
}

func TestSetupUpdateShowsVersionsAndGitWarning(t *testing.T) {
	f := newFakeEnv(t, "linux")
	installed := filepath.Join(f.Home, ".vitral", "bin", "vitral")
	writeFile(t, installed, "binary v-old")
	f.versions["binary v-old"] = "0.0.1"
	f.gitFiles = ".mcp.json"

	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	out := f.out.String()
	if !strings.Contains(out, "versão "+config.Version+" (antes: 0.0.1)") {
		t.Fatalf("update should show old and new versions:\n%s", out)
	}
	if !strings.Contains(out, filepath.Join(f.Cwd, ".mcp.json")+" está no git") || !strings.Contains(out, "vitral setup --global") {
		t.Fatalf("versioned .mcp.json should suggest --global:\n%s", out)
	}
	if readFile(t, installed) != "binary v-new" {
		t.Fatal("binary not replaced")
	}
	if _, err := os.Stat(installed + ".old"); err == nil {
		t.Fatal("old binary should be cleaned up")
	}
}

func TestSetupGlobal(t *testing.T) {
	f := newFakeEnv(t, "darwin")
	global := filepath.Join(f.Home, ".copilot", "mcp-config.json")
	// Sem terminal e sem --yes: mostra o caminho e não grava.
	err := Setup(f.Env, SetupOptions{Global: true})
	if err == nil || !strings.Contains(err.Error(), "precisa de confirmação") || !strings.Contains(f.out.String(), global) {
		t.Fatalf("should print the path and ask for confirmation: %v\n%s", err, f.out.String())
	}
	if _, err := os.Stat(global); err == nil {
		t.Fatal("must not write without confirmation")
	}

	// Interativo respondendo "n": nada muda.
	f.Interactive, f.Stdin = true, strings.NewReader("n\n")
	if err := Setup(f.Env, SetupOptions{Global: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(global); err == nil || !strings.Contains(f.out.String(), "Nada foi alterado") {
		t.Fatal("answer no must not write")
	}

	// Sem VS Code: grava o servidor e a skill, e avisa dos prompts.
	f.out.Reset()
	if err := Setup(f.Env, SetupOptions{Global: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := ReadServer([]byte(readFile(t, global)), KeyPortable, ServerName); !ok {
		t.Fatal("global mcp-config.json not written")
	}
	if !strings.Contains(f.out.String(), "Não encontrei o VS Code") {
		t.Fatalf("%s", f.out.String())
	}

	// Com VS Code (perfil padrão e um perfil) no formato antigo: prompts, migração e skill.
	userDir := filepath.Join(f.Home, "Library", "Application Support", "Code", "User")
	legacyUser := filepath.Join(userDir, "mcp.json")
	legacyProfile := filepath.Join(userDir, "profiles", "abc123", "mcp.json")
	writeFile(t, legacyUser, `{"servers": {"vitral": {"type": "stdio", "command": "/old"}}}`)
	writeFile(t, legacyProfile, `{"servers": {"vitral": {"type": "stdio", "command": "/old"}, "outro": {"command": "x"}}}`)
	f.out.Reset()
	if err := Setup(f.Env, SetupOptions{Global: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{legacyUser, legacyProfile} {
		if strings.Contains(readFile(t, p), "vitral") {
			t.Errorf("%s still has vitral:\n%s", p, readFile(t, p))
		}
	}
	if !strings.Contains(readFile(t, legacyProfile), `"outro"`) {
		t.Error("other servers must stay")
	}
	if prompts, _ := os.ReadDir(filepath.Join(userDir, "prompts")); len(prompts) != 5 {
		t.Fatalf("global prompts = %d", len(prompts))
	}
	if v := SkillVersion([]byte(readFile(t, filepath.Join(f.Home, ".copilot", "skills", "vitral", "SKILL.md")))); v != config.Version {
		t.Fatalf("global skill version %q", v)
	}

	// $COPILOT_HOME muda o lugar do arquivo global.
	f.Getenv = func(k string) string {
		if k == "COPILOT_HOME" {
			return filepath.Join(f.Home, "copilot-home")
		}
		return ""
	}
	if got := PortableGlobal(f.Env).Path; got != filepath.Join(f.Home, "copilot-home", "mcp-config.json") {
		t.Fatalf("COPILOT_HOME: %s", got)
	}
}

func TestVSCodeCandidates(t *testing.T) {
	env := func(vals map[string]string) func(string) string { return func(k string) string { return vals[k] } }
	tests := []struct {
		goos   string
		getenv map[string]string
		want   string
	}{
		{"darwin", nil, filepath.Join("/h", "Library", "Application Support", "Code", "User")},
		{"linux", nil, filepath.Join("/h", ".config", "Code", "User")},
		{"linux", map[string]string{"XDG_CONFIG_HOME": "/x"}, filepath.Join("/x", "Code", "User")},
		{"windows", map[string]string{"APPDATA": `C:\Users\a\AppData\Roaming`}, filepath.Join(`C:\Users\a\AppData\Roaming`, "Code", "User")},
	}
	for _, tt := range tests {
		got := VSCodeCandidates(tt.goos, "/h", env(tt.getenv))
		if got[0].UserDir != tt.want || !strings.Contains(got[1].UserDir, "Code - Insiders") {
			t.Errorf("%s: %+v", tt.goos, got)
		}
	}
	if BinaryName("windows") != "vitral.exe" || BinaryName("linux") != "vitral" {
		t.Fatal("binary name")
	}
}

func TestParseVersionAndDownloadLogger(t *testing.T) {
	if ParseVersion("vitral 1.2.3\n") != "1.2.3" || ParseVersion("outro 1") != "?" || ParseVersion("") != "?" {
		t.Fatal("ParseVersion")
	}
	var buf bytes.Buffer
	l := DownloadLogger{W: &buf}
	l.Println("Download:", "https://x")
	l.Println("Progress:", "45%")
	l.Println("Unzip:", "/tmp/x")
	l.Println("Downloaded:", "/tmp/x")
	want := "Baixando o navegador do Vitral (só na primeira vez)…\n" +
		"Baixando o navegador do Vitral (só na primeira vez)… 45%\n" +
		"Preparando o navegador…\nNavegador pronto.\n"
	if buf.String() != want {
		t.Fatalf("got %q", buf.String())
	}
}

// Rodar o setup de novo com a mesma versão (pelo binário baixado, não o instalado) não é "atualizar".
func TestSetupSameVersionFromDownload(t *testing.T) {
	f := newFakeEnv(t, "linux")
	installed := filepath.Join(f.Home, ".vitral", "bin", "vitral")
	writeFile(t, installed, "binary same")
	f.versions["binary same"] = config.Version

	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	out := f.out.String()
	if !strings.Contains(out, "já está instalado em "+installed) || strings.Contains(out, "antes:") {
		t.Fatalf("same version should not look like an update:\n%s", out)
	}
}
