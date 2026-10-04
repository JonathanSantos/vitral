NAME    := vitral
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/JonathanSantos/vitral/internal/config.Version=$(VERSION)
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

# Ferramentas com versão fixada (rodadas via go run, sem instalação global).
GOLANGCI    := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
GOVULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@v1.8.0
QUALITYGATE := go run ./tools/qualitygate
GREMLINS    := go run github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
# Base da comparação da mutação no PR (o CI passa a branch de destino).
MUTATION_BASE ?= main
OUT         := .quality/out

.PHONY: build dist test unit integration integration-cover e2e mutation mutation-all fuzz network visual visual-update \
        matrix install-test demo-cursor lint fmt vuln gate gate-fast check check-fast nightly release-check clean brand cursorlab tokens-baseline

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

# Laboratório do cursor visual (docs/CURSOR.md, seção 10): .quality/out/cursorlab/index.html.
# Ex.: make cursorlab CURSORLAB="-speed slow -seed 2 -set FittsB=120"
cursorlab:
	go run ./tools/cursorlab $(CURSORLAB)

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

# Mutação só nos pacotes puros alterados em relação à base (gate de PR).
mutation:
	$(QUALITYGATE) mutate -base $(MUTATION_BASE) -out $(OUT)/mutation -gremlins "$(GREMLINS)"

# Mutação em todo o escopo (noturno).
mutation-all:
	$(QUALITYGATE) mutate -all -out $(OUT)/mutation -gremlins "$(GREMLINS)"

# Fuzz de verdade, FUZZTIME por alvo (noturno). As sementes já rodam em todo go test.
FUZZTIME ?= 3m
fuzz:
	@for pkg in $$(go list ./internal/...); do \
		for target in $$(go test -list '^Fuzz' $$pkg | grep '^Fuzz'); do \
			echo "fuzz $$pkg $$target"; \
			go test $$pkg -run '^$$' -fuzz "^$$target$$" -fuzztime $(FUZZTIME) || exit 1; \
		done; \
	done

# Regressão visual com navegador: marcações num print real e o selo em cada estado. Os goldens ficam
# em testdata/visual/<sistema>-chrome-<versão> e exigem o Chrome for Testing fixado (o make e2e baixa).
# O golden das marcações em Go puro roda em todo go test (internal/screenshot).
visual:
	go test -count=1 -tags integration,visual -run '^TestVisual$$' ./internal/app/

# Aceita as mudanças visuais (revise os PNGs no diff antes de commitar).
visual-update:
	go test -count=1 -tags integration,visual -run '^TestVisual$$' ./internal/app/ -update
	go test -count=1 -run '^TestMarksGolden$$' ./internal/screenshot/ -update

# Demonstração do cursor visual (docs/demo.md): janela visível, cursor em slow, roteiro na
# cursor-playground. Fora do gate; serve para avaliar o movimento e gravar o GIF do README.
CURSOR_SEED ?= 1
demo-cursor:
	VITRAL_DEMO_SEED=$(CURSOR_SEED) VITRAL_E2E_VISIBLE=1 VITRAL_E2E_OUT=$(abspath $(OUT)/demo) go test -count=1 -tags 'e2e demo' -run '^TestDemoCursor$$' -v ./test/e2e/

# Matriz: compila o binário e checa os testes (todas as tags) para Linux, macOS e Windows. Rodar os
# testes em cada sistema depende de máquinas de cada um (pendente com o noturno).
MATRIX_OS := linux darwin windows
matrix:
	@for os in $(MATRIX_OS); do \
		echo "matrix $$os"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=amd64 go build -o /dev/null ./cmd/vitral || exit 1; \
		GOOS=$$os GOARCH=amd64 go vet -tags integration,e2e,visual,install,network,eval,demo ./... || exit 1; \
	done

# Instalação e atualização com binários reais (setup, setup de novo, atualização, doctor).
install-test:
	go test -count=1 -tags install ./test/install/

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
	$(QUALITYGATE) -unit $(OUT)/unit.out -combined $(OUT)/integration.out,$(OUT)/e2e.out -tokens $(OUT)/tokens.json -mutation $(OUT)/mutation

# Atualiza a base de tokens com a última medição do e2e (o diff aparece no PR).
tokens-baseline:
	cp $(OUT)/tokens.json .quality/token-baseline.json

# pre-push local (.githooks/pre-push): lint + unitários com race + cobertura unitária.
check-fast: lint unit gate-fast

# Gate de PR (docs/TESTING.md, seção 12). O CI só chama este alvo.
check: lint vuln unit integration-cover e2e mutation gate

# Pendente: o agendamento do noturno ainda não existe (docs/TESTING.md, seção 12).
nightly: check mutation-all fuzz network visual matrix

# Antes de release: noturno + instalação e atualização (rodar em cada um dos 3 sistemas).
release-check: nightly install-test

clean:
	rm -rf bin dist $(OUT)
