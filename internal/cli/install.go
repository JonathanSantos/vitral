package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JonathanSantos/vitral/internal/config"
)

// InstallDir é ~/.vitral/bin: um lugar fixo que não quebra se a pessoa apagar o arquivo baixado.
func InstallDir(home string) string {
	return filepath.Join(home, "."+config.Name, "bin")
}

// BinaryName inclui .exe no Windows.
func BinaryName(goos string) string {
	if goos == "windows" {
		return config.Name + ".exe"
	}
	return config.Name
}

// InstalledPath é o caminho absoluto do binário instalado.
func InstalledPath(e Env) string {
	return filepath.Join(InstallDir(e.Home), BinaryName(e.GOOS))
}

// installResult descreve o que a instalação fez.
type installResult struct {
	Path       string
	OldVersion string // vazio na primeira instalação
	Copied     bool
}

// installBinary copia o binário atual para ~/.vitral/bin, substituindo uma versão anterior.
func installBinary(e Env) (installResult, error) {
	dst := InstalledPath(e)
	res := installResult{Path: dst}
	if _, err := os.Stat(dst); err == nil {
		res.OldVersion = binaryVersion(e, dst)
	}
	if samePath(e.Executable, dst) {
		return res, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return res, err
	}
	tmp := dst + ".new"
	if err := copyFile(e.Executable, tmp); err != nil {
		return res, err
	}
	// Renomear antes funciona mesmo com o Vitral antigo em execução (no Windows não dá para sobrescrever).
	old := dst + ".old"
	_ = os.Remove(old)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			_ = os.Remove(tmp)
			return res, err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		return res, err
	}
	_ = os.Remove(old)
	res.Copied = true
	return res, nil
}

func samePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// binaryVersion roda "<bin> version" e devolve só o número ("?" se não der para ler).
func binaryVersion(e Env, bin string) string {
	out, err := e.Run("", bin, "version")
	if err != nil {
		return "?"
	}
	return ParseVersion(out)
}

// ParseVersion extrai a versão de "vitral 1.2.3".
func ParseVersion(out string) string {
	fields := strings.Fields(out)
	if len(fields) < 2 || fields[0] != config.Name {
		return "?"
	}
	return fields[1]
}

func printInstall(e Env, res installResult) {
	switch {
	case res.OldVersion == "":
		fmt.Fprintf(e.Stdout, msgInstalledFirst, res.Path, config.Version)
	case !res.Copied:
		fmt.Fprintf(e.Stdout, msgInstalledSame, res.Path, config.Version)
	default:
		fmt.Fprintf(e.Stdout, msgInstalledUpdate, res.Path, config.Version, res.OldVersion)
	}
}
