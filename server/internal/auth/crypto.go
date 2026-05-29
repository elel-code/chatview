package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

var publicKeyDecoders = []func(string) ([]byte, error){
	hex.DecodeString,
	base64.StdEncoding.DecodeString,
	base64.RawStdEncoding.DecodeString,
}

func RandomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func NewToken() string {
	return rand.Text()
}

func ParseEd25519PublicKey(pubKey string) (ed25519.PublicKey, error) {
	pubKey = strings.TrimSpace(pubKey)
	if pubKey == "" {
		return nil, errors.New("empty public_key")
	}
	for _, decode := range publicKeyDecoders {
		if raw, err := decode(pubKey); err == nil && len(raw) == ed25519.PublicKeySize {
			return ed25519.PublicKey(raw), nil
		}
	}
	return nil, errors.New("invalid public_key format")
}
