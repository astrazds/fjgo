package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	tag := os.Getenv("TAG")
	if tag == "" {
		fmt.Fprintln(os.Stderr, "TAG is required")
		os.Exit(1)
	}
	md, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	text := string(md)
	start := strings.Index(text, "## "+tag)
	if start < 0 {
		fmt.Fprintf(os.Stderr, "CHANGELOG.md has no section for %s\n", tag)
		os.Exit(1)
	}
	rest := text[start:]
	end := strings.Index(rest[1:], "\n## ")
	notes := rest
	if end >= 0 {
		notes = rest[:end+1]
	}
	if err := os.WriteFile("/tmp/release-notes.md", []byte(strings.TrimSpace(notes)+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
