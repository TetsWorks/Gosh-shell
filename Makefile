BINARY    := gosh
CMD       := ./cmd/gosh
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
BUILD     := $(shell date +%Y%m%d-%H%M%S)
LDFLAGS   := -ldflags "-X main.Version=$(VERSION) -X main.Build=$(BUILD)"

.PHONY: all build install clean test fmt lint run termux

all: build

## build: Compila o gosh
build:
	@echo "→ Compilando gosh $(VERSION)..."
	go build $(LDFLAGS) -o $(BINARY) $(CMD)
	@echo "✓ Binário: ./$(BINARY)"

## install: Instala em /usr/local/bin (ou $PREFIX/bin no Termux)
install: build
	@if [ -n "$$PREFIX" ]; then \
		cp $(BINARY) $$PREFIX/bin/$(BINARY); \
		echo "✓ Instalado em $$PREFIX/bin/$(BINARY)"; \
	else \
		cp $(BINARY) /usr/local/bin/$(BINARY); \
		echo "✓ Instalado em /usr/local/bin/$(BINARY)"; \
	fi

## termux: Build e instala para Termux
termux: build
	cp $(BINARY) $(PREFIX)/bin/$(BINARY)
	@echo "✓ Instalado no Termux"
	@echo "  Execute: gosh"

## run: Executa diretamente
run: build
	./$(BINARY)

## test: Roda todos os testes
test:
	go test ./... -v -count=1

## test-cover: Roda testes com coverage
test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Relatório: coverage.html"

## bench: Benchmarks
bench:
	go test ./... -bench=. -benchmem

## fmt: Formata o código
fmt:
	gofmt -w .
	goimports -w . 2>/dev/null || true

## lint: Linting
lint:
	golangci-lint run ./... 2>/dev/null || go vet ./...

## clean: Remove artefatos
clean:
	rm -f $(BINARY) coverage.out coverage.html
	go clean

## deps: Baixa dependências
deps:
	go mod tidy
	go mod download

## shell-test: Testa features básicas do shell
shell-test: build
	@echo "=== Testando features básicas ==="
	@echo "--- echo ---"
	echo 'echo "hello world"' | ./$(BINARY) -s
	@echo "--- aritmética ---"
	echo 'echo $$(( 2 + 2 ))' | ./$(BINARY) -s
	@echo "--- if/then ---"
	echo 'if [ 1 -eq 1 ]; then echo "ok"; fi' | ./$(BINARY) -s
	@echo "--- for loop ---"
	echo 'for i in 1 2 3; do echo $$i; done' | ./$(BINARY) -s
	@echo "--- pipe ---"
	echo 'echo "hello world" | tr a-z A-Z' | ./$(BINARY) -s
	@echo "=== Todos os testes passaram ==="

## help: Exibe esta ajuda
help:
	@echo "gosh - Go Shell"
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## /  /'
