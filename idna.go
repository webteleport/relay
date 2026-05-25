package relay

import (
	"log"

	"golang.org/x/net/idna"
)

// ToIdna converts a string to its idna form at best effort
// Should only be used on the hostname part without port
func ToIdna(s string) string {
	ascii, err := idna.ToASCII(s)
	if err != nil {
		log.Println(err)
		return s
	}
	return ascii
}
