package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

func NewToken() (string, error) {
	b, err := RandomBytes(32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func ParseEd25519PublicKey(pubKey string) (ed25519.PublicKey, error) {
	pubKey = strings.TrimSpace(pubKey)
	if pubKey == "" {
		return nil, errors.New("empty pub_key")
	}
	if raw, err := hex.DecodeString(pubKey); err == nil && len(raw) == ed25519.PublicKeySize {
		return ed25519.PublicKey(raw), nil
	}
	if raw, err := base64.StdEncoding.DecodeString(pubKey); err == nil && len(raw) == ed25519.PublicKeySize {
		return ed25519.PublicKey(raw), nil
	}
	if raw, err := base64.RawStdEncoding.DecodeString(pubKey); err == nil && len(raw) == ed25519.PublicKeySize {
		return ed25519.PublicKey(raw), nil
	}
	return nil, errors.New("invalid pub_key format")
}
