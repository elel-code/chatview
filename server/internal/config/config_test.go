package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "server.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadUsesYamlOnly(t *testing.T) {
	t.Setenv("CHATVIEW_DB_DSN", "postgres://env")
	t.Setenv("CHATVIEW_LISTEN_ADDR", ":60000")

	cfg, err := Load(writeConfig(t, `
listen_addr: ":50052"
db_dsn: "postgres://file"
tls_cert: "cert.pem"
tls_key: "key.pem"
session_ttl: 12h
challenge_ttl: 2m
cleanup_interval: 3m
presence_heal_interval: 4m
admin_pub_key: "admin"
`))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.ListenAddr != ":50052" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DBDSN != "postgres://file" {
		t.Fatalf("DBDSN = %q", cfg.DBDSN)
	}
	if cfg.TLSCert != "cert.pem" || cfg.TLSKey != "key.pem" {
		t.Fatalf("TLS = %q/%q", cfg.TLSCert, cfg.TLSKey)
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Fatalf("SessionTTL = %s", cfg.SessionTTL)
	}
	if cfg.ChallengeTTL != 2*time.Minute {
		t.Fatalf("ChallengeTTL = %s", cfg.ChallengeTTL)
	}
	if cfg.CleanupInterval != 3*time.Minute {
		t.Fatalf("CleanupInterval = %s", cfg.CleanupInterval)
	}
	if cfg.PresenceHealInterval != 4*time.Minute {
		t.Fatalf("PresenceHealInterval = %s", cfg.PresenceHealInterval)
	}
	if cfg.AdminPubKey != "admin" {
		t.Fatalf("AdminPubKey = %q", cfg.AdminPubKey)
	}
}

func TestLoadRequiresDBDSN(t *testing.T) {
	if _, err := Load(writeConfig(t, `listen_addr: ":50052"`)); err == nil {
		t.Fatal("expected missing db_dsn error")
	}
}
