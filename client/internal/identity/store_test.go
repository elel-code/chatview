package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreCreateExportImportAndCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity-go.bin")
	store := NewStore(path)

	if store.Exists() {
		t.Fatal("new store should not have a local identity")
	}

	created, err := store.Create("123456")
	if err != nil {
		t.Fatal(err)
	}
	if created.PublicKey == "" || created.PrivateKey == "" {
		t.Fatalf("expected exported key material, got %#v", created)
	}
	if !store.Exists() {
		t.Fatal("created identity file was not detected")
	}

	if _, err := store.ExportPrivateKey("000000"); err == nil || err.Error() != "wrong pin" {
		t.Fatalf("expected wrong pin error, got %v", err)
	}

	exported, err := store.ExportPrivateKey("123456")
	if err != nil {
		t.Fatal(err)
	}
	if exported != created.PrivateKey {
		t.Fatalf("exported private key changed: %q != %q", exported, created.PrivateKey)
	}

	if err := store.Import(created.PrivateKey, "654321"); err != nil {
		t.Fatal(err)
	}
	reexported, err := store.ExportPrivateKey("654321")
	if err != nil {
		t.Fatal(err)
	}
	if reexported != created.PrivateKey {
		t.Fatalf("reexported private key changed: %q != %q", reexported, created.PrivateKey)
	}

	if err := os.WriteFile(path, []byte("not-a-chatview-identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExportPrivateKey("654321"); err == nil || !strings.Contains(err.Error(), "identity file") {
		t.Fatalf("expected corrupt identity error, got %v", err)
	}
}

func TestStoreImportAcceptsSeedAndRejectsInvalidKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity-go.bin")
	store := NewStore(path)

	created, err := store.Create("123456")
	if err != nil {
		t.Fatal(err)
	}
	seedHex := created.PrivateKey[:64]
	if err := store.Import(seedHex, "seed-pin"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExportPrivateKey("seed-pin"); err != nil {
		t.Fatal(err)
	}

	if err := store.Import("abcd", "123456"); err == nil {
		t.Fatal("expected invalid private key to fail")
	}
}
