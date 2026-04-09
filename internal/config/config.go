package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	MQTT     MQTTConfig     `yaml:"mqtt"`
	Database DatabaseConfig `yaml:"database"`
	Device   DeviceConfig   `yaml:"device"`
	Auth     AuthConfig     `yaml:"auth"`
}

type ServerConfig struct {
	HTTPAddr string `yaml:"http_port"`
	WSAddr   string `yaml:"ws_port"`
}

type MQTTConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type DatabaseConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

type DeviceConfig struct {
	HeartbeatInterval int `yaml:"heartbeat_interval"`
	OfflineThreshold  int `yaml:"offline_threshold"`
}

type AuthConfig struct {
	JWTSecret          string        `yaml:"jwt_secret"`
	AccessTokenExpire  time.Duration `yaml:"access_token_expire"`
	RefreshTokenExpire time.Duration `yaml:"refresh_token_expire"`
	BcryptCost         int           `yaml:"bcrypt_cost"`
	DefaultAdmin       string        `yaml:"default_admin"`
	DefaultPassword    string        `yaml:"default_password"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Auth.AccessTokenExpire == 0 {
		cfg.Auth.AccessTokenExpire = 30 * time.Minute
	}
	if cfg.Auth.RefreshTokenExpire == 0 {
		cfg.Auth.RefreshTokenExpire = 7 * 24 * time.Hour
	}
	if cfg.Auth.BcryptCost == 0 {
		cfg.Auth.BcryptCost = 10
	}

	return &cfg, nil
}
