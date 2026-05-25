package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

type Options struct {
	DataDir                   string
	GRPCTarget                string
	GRPCUseTLS                bool
	GRPCCACertPath            string
	GRPCSSLTargetNameOverride string
}

func Default() Options {
	target := "127.0.0.1:50051"
	return Options{
		DataDir:    defaultDataDir(),
		GRPCTarget: target,
		GRPCUseTLS: defaultUseTLS(target),
	}
}

func Load(path string) (Options, error) {
	options := Default()
	if strings.TrimSpace(path) == "" {
		return options, nil
	}

	payload, err := os.ReadFile(path)
	if err != nil {
		return options, err
	}

	var file configFile
	if err := yaml.Unmarshal(payload, &file); err != nil {
		return options, err
	}
	if file.DataDir != nil {
		options.DataDir = *file.DataDir
	}
	if file.GRPCTarget != nil {
		options.GRPCTarget = *file.GRPCTarget
		options.GRPCUseTLS = defaultUseTLS(*file.GRPCTarget)
	}
	if file.GRPCUseTLS != nil {
		options.GRPCUseTLS = *file.GRPCUseTLS
	}
	if file.GRPCCACertPath != nil {
		options.GRPCCACertPath = *file.GRPCCACertPath
	}
	if file.GRPCSSLTargetNameOverride != nil {
		options.GRPCSSLTargetNameOverride = *file.GRPCSSLTargetNameOverride
	}
	return options, nil
}

func (o Options) IdentityPath() string {
	return filepath.Join(o.DataDir, "identity-go.bin")
}

func (o Options) CachePath() string {
	return filepath.Join(o.DataDir, "cache-go.db")
}

type configFile struct {
	DataDir                   *string `yaml:"data_dir"`
	GRPCTarget                *string `yaml:"grpc_target"`
	GRPCUseTLS                *bool   `yaml:"grpc_tls"`
	GRPCCACertPath            *string `yaml:"grpc_ca_path"`
	GRPCSSLTargetNameOverride *string `yaml:"grpc_ssl_target_name_override"`
}

func defaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "chatview")
		}
	case "darwin":
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, "Library", "Application Support", "chatview")
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".chatview")
	}
	wd, err := os.Getwd()
	if err != nil {
		return ".chatview"
	}
	return filepath.Join(wd, ".chatview")
}

func defaultUseTLS(target string) bool {
	return !(strings.HasPrefix(target, "localhost:") ||
		strings.HasPrefix(target, "127.") ||
		strings.HasPrefix(target, "[::1]:") ||
		strings.HasPrefix(target, "::1:"))
}
