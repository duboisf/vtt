APP := vocis
VERSION := $(shell git describe --tags --always)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build test fmt tidy

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/vocis

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy

