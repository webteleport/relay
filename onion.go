package relay

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base32"
	"io"
	"strings"

	"golang.org/x/crypto/sha3"
)

func deriveOnionID(p string) string {
	var reader io.Reader
	if p != "/" && p != "" {
		reader = hashedReader(p)
	}
	publicKey, _, _ := ed25519.GenerateKey(reader)
	onionAddress := encodePublicKey(publicKey)
	return onionAddress
}

func encodePublicKey(publicKey ed25519.PublicKey) string {

	// checksum = H(".onion checksum" || pubkey || version)
	var checksumBytes bytes.Buffer
	checksumBytes.Write([]byte(".onion checksum"))
	checksumBytes.Write([]byte(publicKey))
	checksumBytes.Write([]byte{0x03})
	checksum := sha3.Sum256(checksumBytes.Bytes())

	// onion_address = base32(pubkey || checksum || version)
	var onionAddressBytes bytes.Buffer
	onionAddressBytes.Write([]byte(publicKey))
	onionAddressBytes.Write([]byte(checksum[:2]))
	onionAddressBytes.Write([]byte{0x03})
	onionAddress := base32.StdEncoding.EncodeToString(onionAddressBytes.Bytes())

	return strings.ToLower(onionAddress)

}

func hashedReader(s string) io.Reader {
	// Hash the seed to get a 32 byte seed using SHA256
	hash := sha256.New()
	hash.Write([]byte(s))
	return bytes.NewReader(hash.Sum(nil))
}
