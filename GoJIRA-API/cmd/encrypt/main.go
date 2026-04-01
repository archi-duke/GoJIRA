package main

import (
	"fmt"
	"os"

	"gojira-api/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/encrypt/main.go <plaintext>")
		os.Exit(1)
	}
	encrypted, err := config.Encrypt(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "encrypt error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(encrypted)
}
