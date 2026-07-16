package main

import (
	"bytes"
	"os"
)

func main() {
	_, _ = os.Stdout.Write([]byte("ok"))
	_, _ = os.Stderr.Write(bytes.Repeat([]byte("x"), (1<<20)+1024))
	_, _ = os.Stderr.Write([]byte("fjgo-benchmark-credential-canary"))
}
