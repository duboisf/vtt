package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

func readSecret() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}

	if (stat.Mode() & os.ModeCharDevice) == 0 {
		const maxKeySize = 1024
		buf, err := io.ReadAll(io.LimitReader(os.Stdin, maxKeySize))
		if err != nil {
			return "", err
		}
		defer clearBytes(buf)
		return strings.TrimSpace(string(buf)), nil
	}

	fmt.Fprint(os.Stderr, "OpenAI API key: ")
	buf, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	defer clearBytes(buf)
	return strings.TrimSpace(string(buf)), nil
}

func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func findExecutable(name string) (string, bool) {
	path, err := exec.LookPath(name)
	return path, err == nil
}
