package cli

import (
	"fmt"
	"io"
)

// DownloadLogger traduz o progresso do download do Chromium (rod/fetchup) para PT-BR, em stderr.
type DownloadLogger struct{ W io.Writer }

// Println recebe eventos como ("Progress:", "45%").
func (l DownloadLogger) Println(args ...any) {
	if len(args) == 0 {
		return
	}
	event := fmt.Sprint(args[0])
	switch event {
	case "Download:":
		fmt.Fprint(l.W, msgDownloadStart)
	case "Progress:":
		if len(args) > 1 {
			fmt.Fprintf(l.W, msgDownloadProgress, fmt.Sprint(args[1]))
		}
	case "Unzip:":
		fmt.Fprint(l.W, msgDownloadUnzip)
	case "Downloaded:":
		fmt.Fprint(l.W, msgDownloadDone)
	}
}
