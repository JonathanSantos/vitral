package heal

import (
	"testing"

	"go.uber.org/goleak"
)

// Nenhuma goroutine pode sobrar depois dos testes (docs/TESTING.md, seção 9).
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
