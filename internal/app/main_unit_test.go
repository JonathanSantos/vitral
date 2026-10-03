//go:build !integration

package app_test

import (
	"testing"

	"go.uber.org/goleak"
)

// Sem integração (só o contrato): nenhuma goroutine pode sobrar.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
