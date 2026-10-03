// Package tokens tem a estimativa única de tokens usada pelos testes e pelo gate
// (docs/TESTING.md, seção 6). Não é um tokenizer: é uma régua estável para comparar versões.
package tokens

// Estimate devolve ceil(len(texto)/4), com len em bytes (texto UTF-8 como o MCP envia).
func Estimate(text string) int {
	return (len(text) + 3) / 4
}
