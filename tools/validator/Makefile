BINARY := bin/policystack-validator
PKG := ./cmd/policystack-validator

.PHONY: build test lint clean run tidy

build:
	go build -trimpath -o $(BINARY) $(PKG)

test:
	go test ./internal/...

lint:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf bin

run: build
	./$(BINARY) --repo-root ../..
