// Command mcpbridge sobe o Vitral (binário real, MCP por stdio) e expõe as ferramentas por HTTP local,
// para um agente que não tem o Vitral como servidor MCP conseguir usá-lo (simulações e avaliação com
// modelo real). Não entra no binário do Vitral.
//
//	go run ./tools/mcpbridge -addr 127.0.0.1:7801 -log transcript.md -- bin/vitral --cursor-speed normal
//
//	GET  /tools                          lista de ferramentas (nome, descrição, schema)
//	POST /call {"name": "...", "arguments": {...}}  resultado em texto; imagens viram arquivos PNG
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7801", "HTTP address (localhost only)")
	logPath := flag.String("log", "", "append every call and result to this Markdown file")
	shots := flag.String("shots", os.TempDir(), "directory for screenshot PNGs")
	flag.Parse()
	if flag.NArg() == 0 {
		log.Fatal("usage: mcpbridge [flags] -- <vitral binary> [vitral flags]")
	}
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "mcpbridge", Version: "0"}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(flag.Arg(0), flag.Args()[1:]...)}, nil)
	if err != nil {
		log.Fatal(err)
	}

	var mu sync.Mutex // uma chamada por vez, como um agente faria
	step := 0
	record := func(format string, args ...any) {
		if *logPath == "" {
			return
		}
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, format, args...)
	}

	http.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		res, err := cs.ListTools(r.Context(), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		type tool struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema any    `json:"inputSchema"`
		}
		var out []tool
		for _, t := range res.Tools {
			out = append(out, tool{t.Name, t.Description, t.InputSchema})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	http.HandleFunc("/call", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "body must be {\"name\": ..., \"arguments\": {...}}", http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		step++
		start := time.Now()
		cctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		res, err := cs.CallTool(cctx, &mcp.CallToolParams{Name: req.Name, Arguments: req.Arguments})
		if err != nil {
			http.Error(w, "protocol error: "+err.Error(), http.StatusBadGateway)
			return
		}
		var parts []string
		for _, c := range res.Content {
			switch v := c.(type) {
			case *mcp.TextContent:
				parts = append(parts, v.Text)
			case *mcp.ImageContent:
				path := filepath.Join(*shots, fmt.Sprintf("shot-%02d.png", step))
				if err := os.WriteFile(path, v.Data, 0o600); err == nil {
					parts = append(parts, "[image saved to "+path+"]")
				}
			}
		}
		text := strings.Join(parts, "\n")
		if res.IsError {
			text = "ERROR: " + text
		}
		args, _ := json.Marshal(req.Arguments)
		record("### %d. `%s` %s (%.1fs)\n\n```\n%s\n```\n\n", step, req.Name, args, time.Since(start).Seconds(), text)
		fmt.Fprintln(w, text)
	})

	log.Printf("mcpbridge listening on http://%s", *addr)
	srv := &http.Server{Addr: *addr, ReadHeaderTimeout: 10 * time.Second}
	err = srv.ListenAndServe()
	// Fecha o Vitral (e o navegador dele) antes de sair.
	_ = cs.Close()
	log.Fatal(err)
}
