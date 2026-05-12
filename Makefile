BINARY := continuum-plugin-ebooks
GO ?= go

.PHONY: build test clean
build:
	cd web && pnpm install --frozen-lockfile && pnpm run build && cd ..
	$(GO) build -o $(BINARY) ./cmd/continuum-plugin-ebooks
test:
	$(GO) test ./...
clean:
	rm -f $(BINARY)
	rm -rf web/dist
