// Command vapid-key prints a new Web Push key pair for /etc/raid-website.env
// (FR-32, SC-054). The private key goes into the env file only; the public key
// is printed for reference, the server derives it itself.
package main

import (
	"fmt"
	"log"

	"raid-website/internal/push"
)

func main() {
	privateKey, err := push.GenerateVAPIDKey()
	if err != nil {
		log.Fatal(err)
	}
	key, err := push.ParseVAPIDKey(privateKey, "mailto:unused@example.org")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("VAPID_PRIVATE_KEY=%s\n", privateKey)
	fmt.Printf("# public key (applicationServerKey): %s\n", key.PublicKey())
}
