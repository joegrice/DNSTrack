package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Provider struct {
	Name   string   `yaml:"name"`
	IPs    []string `yaml:"ips"`
	Type   string   `yaml:"type"`
	DoHURL string   `yaml:"doh_url,omitempty"`
}

type Config struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Schedule struct {
		Interval      string `yaml:"interval"`
		RetentionDays int    `yaml:"retention_days"`
	} `yaml:"schedule"`
	Domains   []string   `yaml:"domains"`
	Providers []Provider `yaml:"providers"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8420
	}
	if cfg.Schedule.Interval == "" {
		cfg.Schedule.Interval = "@every 1h"
	}
	if cfg.Schedule.RetentionDays == 0 {
		cfg.Schedule.RetentionDays = 90
	}

	return cfg, nil
}
