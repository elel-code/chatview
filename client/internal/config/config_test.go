package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadYAMLConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	if err := os.WriteFile(path, []byte(`
data_dir: "/tmp/chatview-client"
grpc_target: "chatview.example.com:443"
grpc_tls: false
grpc_ca_path: "/tmp/ca.pem"
grpc_ssl_target_name_override: "dev.chatview.local"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	options, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if options.DataDir != "/tmp/chatview-client" {
		t.Fatalf("unexpected data dir: %q", options.DataDir)
	}
	if options.GRPCTarget != "chatview.example.com:443" {
		t.Fatalf("unexpected target: %q", options.GRPCTarget)
	}
	if options.GRPCUseTLS {
		t.Fatal("explicit grpc_tls=false should override target default")
	}
	if options.GRPCCACertPath != "/tmp/ca.pem" {
		t.Fatalf("unexpected CA path: %q", options.GRPCCACertPath)
	}
	if options.GRPCSSLTargetNameOverride != "dev.chatview.local" {
		t.Fatalf("unexpected target override: %q", options.GRPCSSLTargetNameOverride)
	}
}

func TestLoadDefaultsTLSFromTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	if err := os.WriteFile(path, []byte(`grpc_target: "chatview.example.com:443"`), 0o600); err != nil {
		t.Fatal(err)
	}
	options, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !options.GRPCUseTLS {
		t.Fatal("non-loopback target should default to TLS")
	}

	if err := os.WriteFile(path, []byte(`grpc_target: "127.0.0.1:50051"`), 0o600); err != nil {
		t.Fatal(err)
	}
	options, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if options.GRPCUseTLS {
		t.Fatal("loopback target should default to insecure transport")
	}
}

func TestRuntimePathsUseGoFiles(t *testing.T) {
	options := Options{DataDir: "/tmp/chatview-client"}
	if got := options.IdentityPath(); got != "/tmp/chatview-client/identity-go.bin" {
		t.Fatalf("unexpected identity path: %q", got)
	}
	if got := options.CachePath(); got != "/tmp/chatview-client/cache-go.db" {
		t.Fatalf("unexpected cache path: %q", got)
	}
}
