// Package vitral expõe arquivos do repositório embutidos no binário.
package vitral

import "embed"

// Prompts são os prompt files do Copilot que o "vitral setup" copia para o projeto.
//
//go:embed examples/vscode/.github/prompts/*.prompt.md
var Prompts embed.FS

// PromptsDir é o diretório dos prompts dentro de Prompts.
const PromptsDir = "examples/vscode/.github/prompts"

// Skill é a skill do Copilot (Agent Skills) que o "vitral setup" instala. Fonte única: o setup só
// preenche o marcador de versão (<!-- vitral-skill-version: ... -->) com a versão do binário.
//
//go:embed examples/vscode/.github/skills/vitral/SKILL.md
var Skill string

// ExtensionIcons são os ícones da extensão dos Retoques (a marca, nos tamanhos que o Chrome usa).
//
//go:embed assets/brand/icon-16.png assets/brand/icon-32.png assets/brand/icon-48.png assets/brand/icon-128.png
var ExtensionIcons embed.FS
