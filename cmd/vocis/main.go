package main

import (
	"fmt"
	"os"
)

// version is set at build time via -ldflags:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)" ./cmd/vocis
var version = "dev"

func main() {
	if err := Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
