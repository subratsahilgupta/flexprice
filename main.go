package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
)

// generateKey creates a random 256-bit key for AES-256.
func generateKey() []byte {
	key := make([]byte, 32) // 32 bytes = 256 bits
	if _, err := rand.Read(key); err != nil {
		log.Fatalf("Unable to generate key: %v", err)
	}
	return key
}

func main() {
	// Generate and display the encryption key.
	key := generateKey()
	fmt.Println("Generated Key (hex):", hex.EncodeToString(key))

}
