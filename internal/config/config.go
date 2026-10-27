package config

import (
	"os"

	"github.com/spf13/viper"
)

// Defaults promised by the migration plan.
const (
	DefaultPort = "8082"
	DefaultDSN  = "root:root@tcp(localhost:3306)/barcode?parseTime=true"
)

type Config struct {
	DatabaseURL string `mapstructure:"DATABASE_URL"`
	Port        string `mapstructure:"PORT"`
	JWTSecret   string `mapstructure:"JWT_SECRET"`
}

func Load() (*Config, error) {
	viper.AutomaticEnv()
	// AutomaticEnv() alone does NOT make viper.Unmarshal() populate a
	// mapstructure-tagged field from a real env var unless viper already
	// "knows about" that key first (via SetDefault or an explicit BindEnv).
	// Without these, Unmarshal silently leaves fields empty regardless of
	// what's actually set in the real environment.
	viper.SetDefault("PORT", DefaultPort)
	_ = viper.BindEnv("DATABASE_URL")
	_ = viper.BindEnv("JWT_SECRET")
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// SERVER_PORT and DB_DSN override the configuration.
	if v := os.Getenv("SERVER_PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("DB_DSN"); v != "" {
		cfg.DatabaseURL = v
	}
	if cfg.Port == "" {
		cfg.Port = DefaultPort
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = DefaultDSN
	}
	return &cfg, nil
}
