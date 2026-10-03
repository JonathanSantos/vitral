package install

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// Logger recebe o progresso no formato de eventos já usado pela CLI:
// ("Download:", url), ("Progress:", "45%"), ("Unzip:", dir), ("Downloaded:", dir).
type Logger interface {
	Println(...any)
}

// Installer instala o Chrome for Testing em Root/<versão>.
type Installer struct {
	Root     string // ~/.vitral/browsers
	Version  string
	Platform string
	Mirror   string // base URL alternativa (mesmo layout de paths); vazio = oficial
	Client   *http.Client
	Progress Logger
	// VersionOf roda o executável com --version e devolve a versão (injetável em testes).
	VersionOf func(exe string) (string, error)
}

// NewInstaller cria o instalador padrão, respeitando HTTPS_PROXY/HTTP_PROXY do ambiente.
func NewInstaller(root, platform, mirror string, progress Logger) *Installer {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	return &Installer{
		Root: root, Version: PinnedVersion, Platform: platform, Mirror: mirror,
		Client:    &http.Client{Transport: transport},
		Progress:  progress,
		VersionOf: RunVersion,
	}
}

// Dir é a pasta da versão instalada.
func (i *Installer) Dir() string { return filepath.Join(i.Root, i.Version) }

// Executable é o caminho do Chrome instalado (pode ainda não existir).
func (i *Installer) Executable() string {
	return filepath.Join(i.Dir(), filepath.FromSlash(ExecutableRel(i.Platform)))
}

func (i *Installer) log(args ...any) {
	if i.Progress != nil {
		i.Progress.Println(args...)
	}
}

// DownloadURL: com mirror, monta pelo layout; sem mirror, pergunta ao JSON oficial
// (e cai no layout oficial se o JSON estiver inacessível).
func (i *Installer) DownloadURL(ctx context.Context) (string, error) {
	if i.Mirror != "" {
		return ZipURL(i.Mirror, i.Version, i.Platform), nil
	}
	data, err := i.get(ctx, VersionURL(i.Version))
	if err != nil {
		return ZipURL(DefaultBaseURL, i.Version, i.Platform), nil
	}
	return ParseDownloadURL(data, i.Platform)
}

func (i *Installer) get(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := i.Client.Do(req) // #nosec G704 -- URL da constante oficial ou do mirror configurado pela pessoa
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 4<<20))
}

// Probe faz só um HEAD na URL do zip: diz se a rede deixa baixar, sem baixar.
func (i *Installer) Probe(ctx context.Context) (string, error) {
	url, err := i.DownloadURL(ctx)
	if err != nil {
		return url, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return url, err
	}
	res, err := i.Client.Do(req) // #nosec G704 -- URL da constante oficial ou do mirror configurado pela pessoa
	if err != nil {
		return url, err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return url, fmt.Errorf("HEAD %s: HTTP %d", url, res.StatusCode)
	}
	return url, nil
}

// Install baixa o zip da versão fixada e instala. Devolve o caminho do executável.
func (i *Installer) Install(ctx context.Context) (string, error) {
	url, err := i.DownloadURL(ctx)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(i.Root, 0o750); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(i.Root, ".download-*.zip")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	i.log("Download:", url)
	if err := i.download(ctx, url, tmp); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return i.InstallZip(tmp.Name())
}

func (i *Installer) download(ctx context.Context, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := i.Client.Do(req) // #nosec G704 -- URL da constante oficial ou do mirror configurado pela pessoa
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, res.StatusCode)
	}
	_, err = io.Copy(w, &progressReader{r: res.Body, total: res.ContentLength, log: i.log})
	return err
}

type progressReader struct {
	r     io.Reader
	total int64
	read  int64
	last  time.Time
	log   func(...any)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if p.total > 0 && time.Since(p.last) >= time.Second {
		p.last = time.Now()
		p.log("Progress:", fmt.Sprintf("%02d%%", p.read*100/p.total))
	}
	return n, err
}

// InstallZip extrai numa pasta temporária, valida com --version e só então faz o rename atômico.
func (i *Installer) InstallZip(zipPath string) (string, error) {
	if err := os.MkdirAll(i.Root, 0o750); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(i.Root, ".staging-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)

	i.log("Unzip:", staging)
	if err := Extract(zipPath, staging); err != nil {
		return "", err
	}
	exe := filepath.Join(staging, filepath.FromSlash(ExecutableRel(i.Platform)))
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("the zip is not Chrome for Testing for %s (missing %s)", i.Platform, ExecutableRel(i.Platform))
	}
	got, err := i.VersionOf(exe)
	if err != nil {
		return "", fmt.Errorf("the downloaded browser does not run: %w", err)
	}
	if got != i.Version {
		return "", fmt.Errorf("the zip has Chrome for Testing %s, but Vitral expects %s (%s)", got, i.Version, ZipURL(DefaultBaseURL, i.Version, i.Platform))
	}
	if err := os.RemoveAll(i.Dir()); err != nil {
		return "", err
	}
	if err := os.Rename(staging, i.Dir()); err != nil { // #nosec G703 -- Dir() vem da configuração, não do zip
		return "", err
	}
	i.log("Downloaded:", i.Dir())
	Cleanup(i.Root, i.Version)
	return i.Executable(), nil
}

// Extract descompacta preservando permissões e symlinks (o .app do macOS depende deles)
// e recusa entradas que tentem escapar da pasta (zip-slip).
func Extract(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()
	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	for _, f := range r.File {
		if err := extractOne(f, root); err != nil {
			return err
		}
	}
	return nil
}

// maxEntrySize limita cada arquivo extraído (proteção contra "zip bomb").
const maxEntrySize = 2 << 30

func inside(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func extractOne(f *zip.File, root string) error {
	target := filepath.Join(root, filepath.FromSlash(f.Name))
	if !inside(root, target) {
		return fmt.Errorf("unsafe path in zip: %s", f.Name)
	}
	mode := f.Mode()
	if f.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o750) // #nosec G703 -- target conferido por inside()
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil { // #nosec G703 -- target conferido por inside()
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if mode&os.ModeSymlink != 0 {
		link, err := io.ReadAll(io.LimitReader(rc, 4096))
		if err != nil {
			return err
		}
		dest := string(link)
		if filepath.IsAbs(dest) || !inside(root, filepath.Join(filepath.Dir(target), dest)) {
			return fmt.Errorf("unsafe symlink in zip: %s -> %s", f.Name, dest)
		}
		return os.Symlink(dest, target) // #nosec G703 -- origem e destino conferidos por inside()
	}
	perm := mode.Perm()
	if perm == 0 {
		perm = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm|0o200) // #nosec G703 -- conferido por inside()
	if err != nil {
		return err
	}
	// Limite por arquivo contra zip bomb: o maior arquivo do Chrome tem algumas centenas de MB.
	n, err := io.Copy(out, io.LimitReader(rc, maxEntrySize+1))
	if err == nil && n > maxEntrySize {
		err = fmt.Errorf("zip entry too large: %s", f.Name)
	}
	if err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Cleanup mantém só a versão atual e a anterior mais nova; remove o resto e sobras temporárias.
func Cleanup(root, current string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var others []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".staging-") || strings.HasPrefix(name, ".download-") {
			_ = os.RemoveAll(filepath.Join(root, name))
			continue
		}
		if e.IsDir() && name != current && len(versionParts(name)) >= 2 {
			others = append(others, name)
		}
	}
	slices.SortFunc(others, func(a, b string) int { return CompareVersions(b, a) })
	for _, old := range others[min(1, len(others)):] {
		_ = os.RemoveAll(filepath.Join(root, old))
	}
}

// RunVersion roda "<exe> --version". No Windows o chrome.exe não escreve no stdout,
// então a versão sai do arquivo <versão>.manifest ao lado do executável.
func RunVersion(exe string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if v := ParseVersion(string(out)); err == nil && v != "" {
		return v, nil
	}
	if runtime.GOOS == "windows" {
		if v := manifestVersion(filepath.Dir(exe)); v != "" {
			return v, nil
		}
	}
	if err == nil {
		err = errors.New("no version in --version output")
	}
	return "", err
}

func manifestVersion(dir string) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if v := strings.TrimSuffix(e.Name(), ".manifest"); v != e.Name() && len(versionParts(v)) == 4 {
			return v
		}
	}
	return ""
}
