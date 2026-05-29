package config

import (
	"cmp"
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
	cfg.ListenAddr = cmp.Or(raw.ListenAddr, cfg.ListenAddr)
	cfg.DBDSN = cmp.Or(raw.DBDSN, cfg.DBDSN)
	cfg.TLSCert = cmp.Or(raw.TLSCert, cfg.TLSCert)
	cfg.TLSKey = cmp.Or(raw.TLSKey, cfg.TLSKey)
	cfg.AdminPubKey = cmp.Or(raw.AdminPubKey, cfg.AdminPubKey)
	cfg.MigrationsDir = cmp.Or(raw.MigrationsDir, cfg.MigrationsDir)
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
