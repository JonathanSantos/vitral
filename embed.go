// Package vitral expõe arquivos do repositório embutidos no binário.
package vitral

import "embed"

// Prompts são os prompt files do Copilot que o "vitral setup" copia para o projeto.
//
//go:embed examples/vscode/.github/prompts/*.prompt.md
var Prompts embed.FS

// PromptsDir é o diretório dos prompts dentro de Prompts.
const PromptsDir = "examples/vscode/.github/prompts"
