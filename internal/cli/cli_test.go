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
	load := func([]string) (config.Config, error) { return config.Config{}, nil }
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
	mcp := readFile(t, filepath.Join(f.Cwd, ".vscode", "mcp.json"))
	entry, ok, err := ReadServer([]byte(mcp), ServerName)
	if err != nil || !ok || entry.Command != installed || entry.Type != "stdio" {
		t.Fatalf("mcp.json = %s (%v)", mcp, err)
	}
	prompts, _ := os.ReadDir(filepath.Join(f.Cwd, ".github", "prompts"))
	if len(prompts) != 5 {
		t.Fatalf("want 5 prompts, got %d", len(prompts))
	}
	out := f.out.String()
	for _, want := range []string{"Vitral instalado em", "5 novos", "Próximos passos"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(f.Cwd, ".vscode", "mcp.json.bak")); err == nil {
		t.Error("no backup expected for a new file")
	}
}

func TestSetupMergesAndDoesNotOverwrite(t *testing.T) {
	f := newFakeEnv(t, "linux")
	original := `{
  // configuração da equipe
  "servers": {
    "github": { "type": "http", "url": "https://api.githubcopilot.com/mcp/" },
  },
}`
	writeFile(t, filepath.Join(f.Cwd, ".vscode", "mcp.json"), original)
	writeFile(t, filepath.Join(f.Cwd, ".github", "prompts", "vitral-descrever-tela.prompt.md"), "minha versão")

	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	mcp := readFile(t, filepath.Join(f.Cwd, ".vscode", "mcp.json"))
	for _, want := range []string{"// configuração da equipe", `"github": { "type": "http"`, `"vitral"`} {
		if !strings.Contains(mcp, want) {
			t.Errorf("missing %q in merged mcp.json:\n%s", want, mcp)
		}
	}
	if readFile(t, filepath.Join(f.Cwd, ".vscode", "mcp.json.bak")) != original {
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

func TestSetupUpdateShowsVersionsAndGitWarning(t *testing.T) {
	f := newFakeEnv(t, "linux")
	installed := filepath.Join(f.Home, ".vitral", "bin", "vitral")
	writeFile(t, installed, "binary v-old")
	f.versions["binary v-old"] = "0.0.1"
	f.gitFiles = ".vscode/settings.json"

	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	out := f.out.String()
	if !strings.Contains(out, "versão "+config.Version+" (antes: 0.0.1)") {
		t.Fatalf("update should show old and new versions:\n%s", out)
	}
	if !strings.Contains(out, "vitral setup --global") {
		t.Fatalf("versioned .vscode should suggest --global:\n%s", out)
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
	if err := Setup(f.Env, SetupOptions{Global: true}); err == nil || !strings.Contains(err.Error(), "não encontrei o VS Code") {
		t.Fatalf("no VS Code should fail clearly: %v", err)
	}

	userDir := filepath.Join(f.Home, "Library", "Application Support", "Code", "User")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Sem terminal e sem --yes: mostra o caminho e não grava.
	f.out.Reset()
	err := Setup(f.Env, SetupOptions{Global: true})
	if err == nil || !strings.Contains(f.out.String(), filepath.Join(userDir, "mcp.json")) {
		t.Fatalf("should print the path and ask for confirmation: %v\n%s", err, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(userDir, "mcp.json")); err == nil {
		t.Fatal("must not write without confirmation")
	}

	// Interativo respondendo "n": pula.
	f.Interactive, f.Stdin = true, strings.NewReader("n\n")
	if err := Setup(f.Env, SetupOptions{Global: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(userDir, "mcp.json")); err == nil {
		t.Fatal("answer no must not write")
	}

	// --yes: grava mcp.json e prompts do usuário.
	if err := Setup(f.Env, SetupOptions{Global: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := ReadServer([]byte(readFile(t, filepath.Join(userDir, "mcp.json"))), ServerName); !ok {
		t.Fatal("global mcp.json not written")
	}
	if prompts, _ := os.ReadDir(filepath.Join(userDir, "prompts")); len(prompts) != 5 {
		t.Fatalf("global prompts = %d", len(prompts))
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
