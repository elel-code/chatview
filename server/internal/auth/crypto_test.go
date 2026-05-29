package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestParseEd25519PublicKeyAcceptsSupportedEncodings(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"hex":        hex.EncodeToString(publicKey),
		"base64":     base64.StdEncoding.EncodeToString(publicKey),
		"raw base64": base64.RawStdEncoding.EncodeToString(publicKey),
		"trimmed":    " " + hex.EncodeToString(publicKey) + "\n",
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ParseEd25519PublicKey(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(publicKey) {
				t.Fatalf("parsed key mismatch")
			}
		})
	}
}

func TestParseEd25519PublicKeyRejectsInvalidInput(t *testing.T) {
	tests := []string{"", "   ", "not-a-key", hex.EncodeToString([]byte("short"))}
	for _, input := range tests {
		if _, err := ParseEd25519PublicKey(input); err == nil {
			t.Fatalf("ParseEd25519PublicKey(%q) returned nil error", input)
		}
	}
}

func TestRandomHelpersReturnUsableValues(t *testing.T) {
	if got := RandomBytes(17); len(got) != 17 {
		t.Fatalf("RandomBytes length = %d, want 17", len(got))
	}
	if NewToken() == "" {
		t.Fatal("NewToken returned empty string")
	}
}
