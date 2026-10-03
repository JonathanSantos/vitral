BIN := bin/browser-mcp
STATICCHECK := go run honnef.co/go/tools/cmd/staticcheck@latest

.PHONY: build test integration lint check

build:
	go build -o $(BIN) ./cmd/browser-mcp

test:
	go test ./...

integration:
	go test -tags integration ./...

lint:
	go vet ./... && go vet -tags integration ./...
	$(STATICCHECK) ./... && $(STATICCHECK) -tags integration ./...

check: lint test integration
