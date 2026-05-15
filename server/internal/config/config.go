package config

import (
	"errors"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddr           string        `yaml:"listen_addr"`
	DBDSN                string        `yaml:"db_dsn"`
	TLSCert              string        `yaml:"tls_cert"`
	TLSKey               string        `yaml:"tls_key"`
	SessionTTL           time.Duration `yaml:"session_ttl"`
	ChallengeTTL         time.Duration `yaml:"challenge_ttl"`
	CleanupInterval      time.Duration `yaml:"cleanup_interval"`
	PresenceHealInterval time.Duration `yaml:"presence_heal_interval"`
	AdminPubKey          string        `yaml:"admin_pub_key"`
	MigrationsDir        string        `yaml:"migrations_dir"`
	SkipMigrations       bool          `yaml:"skip_migrations"`
}

func Load(path string) (Config, error) {
	cfg := Config{
		ListenAddr:           ":50051",
		MigrationsDir:        "migrations",
		SessionTTL:           24 * time.Hour,
		ChallengeTTL:         5 * time.Minute,
		CleanupInterval:      10 * time.Minute,
		PresenceHealInterval: 5 * time.Minute,
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		if err := unmarshalYAML(data, &cfg); err != nil {
			return Config{}, err
		}
	}
	if cfg.DBDSN == "" {
		return Config{}, errors.New("db_dsn is required")
	}
	return cfg, nil
}

type fileConfig struct {
	ListenAddr           string `yaml:"listen_addr"`
	DBDSN                string `yaml:"db_dsn"`
	TLSCert              string `yaml:"tls_cert"`
	TLSKey               string `yaml:"tls_key"`
	SessionTTL           string `yaml:"session_ttl"`
	ChallengeTTL         string `yaml:"challenge_ttl"`
	CleanupInterval      string `yaml:"cleanup_interval"`
	PresenceHealInterval string `yaml:"presence_heal_interval"`
	AdminPubKey          string `yaml:"admin_pub_key"`
	MigrationsDir        string `yaml:"migrations_dir"`
	SkipMigrations       bool   `yaml:"skip_migrations"`
}

func unmarshalYAML(data []byte, cfg *Config) error {
	var raw fileConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ListenAddr != "" {
		cfg.ListenAddr = raw.ListenAddr
	}
	if raw.DBDSN != "" {
		cfg.DBDSN = raw.DBDSN
	}
	if raw.TLSCert != "" {
		cfg.TLSCert = raw.TLSCert
	}
	if raw.TLSKey != "" {
		cfg.TLSKey = raw.TLSKey
	}
	if raw.AdminPubKey != "" {
		cfg.AdminPubKey = raw.AdminPubKey
	}
	if raw.MigrationsDir != "" {
		cfg.MigrationsDir = raw.MigrationsDir
	}
	cfg.SkipMigrations = raw.SkipMigrations
	var err error
	if cfg.SessionTTL, err = parseDuration(raw.SessionTTL, cfg.SessionTTL); err != nil {
		return err
	}
	if cfg.ChallengeTTL, err = parseDuration(raw.ChallengeTTL, cfg.ChallengeTTL); err != nil {
		return err
	}
	if cfg.CleanupInterval, err = parseDuration(raw.CleanupInterval, cfg.CleanupInterval); err != nil {
		return err
	}
	if cfg.PresenceHealInterval, err = parseDuration(raw.PresenceHealInterval, cfg.PresenceHealInterval); err != nil {
		return err
	}
	return nil
}

func parseDuration(raw string, fallback time.Duration) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Duration(n), nil
	}
	return time.ParseDuration(raw)
}
