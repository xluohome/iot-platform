package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	MQTT     MQTTConfig     `yaml:"mqtt"`
	Database DatabaseConfig `yaml:"database"`
	Device   DeviceConfig   `yaml:"device"`
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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
