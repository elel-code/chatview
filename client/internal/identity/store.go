package identity

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"crypto/pbkdf2"
)

var fileMagic = []byte{'C', 'H', 'T', 'V', 'I', 'D', 'G', '1'}

const (
	saltSize  = 16
	nonceSize = 12
	keySize   = 32
	pbkdfIter = 210_000
)

type Store struct {
	path string
}

type Identity struct {
	PublicKey  string
	PrivateKey string
}

type Keypair struct {
	PublicHex string
	Private   ed25519.PrivateKey
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

func (s *Store) Create(pin string) (Identity, error) {
	if strings.TrimSpace(pin) == "" {
		return Identity{}, errors.New("pin is required")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, err
	}
	seed := privateKey.Seed()
	if err := s.saveSeed(seed, pin); err != nil {
		return Identity{}, err
	}
	return Identity{
		PublicKey:  hex.EncodeToString(publicKey),
		PrivateKey: hex.EncodeToString(privateKey),
	}, nil
}

func (s *Store) Import(privateKeyHex, pin string) error {
	seed, err := seedFromPrivateHex(privateKeyHex)
	if err != nil {
		return err
	}
	return s.saveSeed(seed, pin)
}

func (s *Store) Load(pin string) (Keypair, error) {
	seed, err := s.loadSeed(pin)
	if err != nil {
		return Keypair{}, err
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return Keypair{}, errors.New("invalid public key")
	}
	return Keypair{
		PublicHex: hex.EncodeToString(publicKey),
		Private:   privateKey,
	}, nil
}

func (s *Store) ExportPrivateKey(pin string) (string, error) {
	keypair, err := s.Load(pin)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(keypair.Private), nil
}

func (s *Store) saveSeed(seed []byte, pin string) error {
	if len(seed) != ed25519.SeedSize {
		return errors.New("invalid seed size")
	}
	salt := make([]byte, saltSize)
	nonce := make([]byte, nonceSize)
	rand.Read(salt)
	rand.Read(nonce)
	key, err := deriveKey(pin, salt)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	ciphertext := aead.Seal(nil, nonce, seed, nil)

	payload := make([]byte, 0, len(fileMagic)+len(salt)+len(nonce)+len(ciphertext))
	payload = append(payload, fileMagic...)
	payload = append(payload, salt...)
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, payload, 0o600)
}

func (s *Store) loadSeed(pin string) ([]byte, error) {
	payload, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	minSize := len(fileMagic) + saltSize + nonceSize
	if len(payload) <= minSize {
		return nil, errors.New("invalid identity file")
	}
	if !bytes.HasPrefix(payload, fileMagic) {
		return nil, errors.New("unsupported identity file format")
	}
	offset := len(fileMagic)
	salt := payload[offset : offset+saltSize]
	offset += saltSize
	nonce := payload[offset : offset+nonceSize]
	offset += nonceSize
	ciphertext := payload[offset:]

	key, err := deriveKey(pin, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	seed, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("wrong pin")
	}
	if len(seed) != ed25519.SeedSize {
		return nil, errors.New("invalid identity payload")
	}
	return seed, nil
}

func deriveKey(pin string, salt []byte) ([]byte, error) {
	if strings.TrimSpace(pin) == "" {
		return nil, errors.New("pin is required")
	}
	return pbkdf2.Key(sha256.New, pin, salt, pbkdfIter, keySize)
}

func seedFromPrivateHex(privateKeyHex string) ([]byte, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(privateKeyHex))
	if err != nil {
		return nil, fmt.Errorf("invalid private key hex: %w", err)
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return raw, nil
	case ed25519.PrivateKeySize:
		return raw[:ed25519.SeedSize], nil
	default:
		return nil, errors.New("private key must be 32-byte seed or 64-byte Ed25519 private key hex")
	}
}
