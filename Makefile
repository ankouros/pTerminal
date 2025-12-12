SHELL := /usr/bin/env bash
BIN := bin/pterminal

.PHONY: build run clean assets fmt vet

build:
	go build -o $(BIN) ./cmd/pterminal

run: build
	./$(BIN)

assets:
	bash scripts/fetch_assets.sh

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin
