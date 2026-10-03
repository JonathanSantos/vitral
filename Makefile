NAME    := browser-mcp
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/jors/browser-mcp/internal/config.Version=$(VERSION)
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
STATICCHECK := go run honnef.co/go/tools/cmd/staticcheck@latest

.PHONY: build dist test integration lint check clean

# Binário para a máquina atual em bin/.
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(NAME) ./cmd/browser-mcp

# Binários para todas as plataformas em dist/.
dist:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; [ $$os = windows ] && ext=".exe"; \
		out=dist/$(NAME)_$(VERSION)_$${os}_$${arch}$$ext; \
		echo "build $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $$out ./cmd/browser-mcp || exit 1; \
	done

test:
	go test ./...

integration:
	go test -tags integration ./...

lint:
	go vet ./... && go vet -tags integration ./...
	$(STATICCHECK) ./... && $(STATICCHECK) -tags integration ./...

check: lint test integration

clean:
	rm -rf bin dist
