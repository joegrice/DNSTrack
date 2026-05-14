package config

import (
	"fmt"
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
	Domains     []string   `yaml:"domains"`
	RecordTypes []string   `yaml:"record_types"`
	Providers   []Provider `yaml:"providers"`
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
	if cfg.RecordTypes == nil || len(cfg.RecordTypes) == 0 {
		cfg.RecordTypes = []string{"A"}
	}

	if len(cfg.Domains) == 0 {
		return nil, fmt.Errorf("config: at least one domain is required")
	}
	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("config: at least one provider is required")
	}

	return cfg, nil
}
