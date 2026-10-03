NAME    := vitral
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/JonathanSantos/vitral/internal/config.Version=$(VERSION)
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

# Ferramentas com versão fixada (rodadas via go run, sem instalação global).
GOLANGCI    := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
GOVULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@v1.8.0
QUALITYGATE := go run ./tools/qualitygate
OUT         := .quality/out

.PHONY: build dist test unit integration integration-cover e2e network lint fmt vuln \
        gate gate-fast check check-fast nightly release-check clean brand tokens-baseline

# ---------- build ----------

# Binário para a máquina atual em bin/.
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(NAME) ./cmd/vitral

# Binários para todas as plataformas em dist/.
dist:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; [ $$os = windows ] && ext=".exe"; \
		out=dist/$(NAME)_$(VERSION)_$${os}_$${arch}$$ext; \
		echo "build $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $$out ./cmd/vitral || exit 1; \
	done

# Regenera os PNGs de assets/brand a partir dos SVGs (usa o Chrome local, funciona offline).
brand:
	go run ./tools/brandgen

# ---------- testes ----------

test: unit

# Unitários com race e cobertura (critério "unit" do thresholds.yaml).
unit:
	@mkdir -p $(OUT)
	go test -race -count=1 -coverprofile=$(OUT)/unit.out ./...

# Integração com browser, só rodando os testes.
integration:
	go test -race -count=1 -tags integration ./...

# Integração com cobertura de todo internal/ (entra na cobertura "combined").
integration-cover:
	@mkdir -p $(OUT)
	go test -race -count=1 -tags integration -coverpkg=./internal/... -coverprofile=$(OUT)/integration.out ./internal/... ./cmd/...

# E2E: binário real por stdio, agentes roteirizado e heurístico, modo caótico (docs/TESTING.md, seção 7).
# O binário é compilado com -cover; a cobertura entra na combinada.
e2e:
	@rm -rf $(OUT)/e2e/cover && mkdir -p $(OUT)/e2e/cover
	VITRAL_E2E_COVERDIR=$(abspath $(OUT)/e2e/cover) go test -tags e2e -count=1 ./test/e2e/
	go tool covdata textfmt -i=$(OUT)/e2e/cover -o=$(OUT)/e2e.out

# Download real do Chrome for Testing (≈150 MB); fora do gate de PR.
network:
	go test -tags network ./internal/browser/install/

# ---------- qualidade ----------

lint:
	$(GOLANGCI) run ./...

fmt:
	$(GOLANGCI) fmt ./...

vuln:
	$(GOVULNCHECK) ./...

# Gate só com a cobertura unitária (pacotes "combined" aparecem como não medidos).
gate-fast:
	$(QUALITYGATE) -unit $(OUT)/unit.out

gate:
	$(QUALITYGATE) -unit $(OUT)/unit.out -combined $(OUT)/integration.out,$(OUT)/e2e.out -tokens $(OUT)/tokens.json

# Atualiza a base de tokens com a última medição do e2e (o diff aparece no PR).
tokens-baseline:
	cp $(OUT)/tokens.json .quality/token-baseline.json

# pre-push local (.githooks/pre-push): lint + unitários com race + cobertura unitária.
check-fast: lint unit gate-fast

# Gate de PR (docs/TESTING.md, seção 12). O CI só chama este alvo.
check: lint vuln unit integration-cover e2e gate

# Pendente: o agendamento do noturno ainda não existe (docs/TESTING.md, seção 12).
nightly: check network

release-check: nightly

clean:
	rm -rf bin dist $(OUT)
