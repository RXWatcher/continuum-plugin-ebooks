BINARY := silo-plugin-ebooks
GO ?= go
PNPM ?= pnpm

.PHONY: build test clean
build:
	cd web && $(PNPM) install --frozen-lockfile && $(PNPM) run build
	$(GO) build -o $(BINARY) ./cmd/silo-plugin-ebooks
test:
	$(GO) test ./...
clean:
	rm -f $(BINARY)
	rm -rf web/dist
