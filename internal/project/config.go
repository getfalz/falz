package project

import (
	"os"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type PublishConfig struct {
	Provider       string `yaml:"provider"`
	FTPCredentials struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"ftp_credentials"`
}

// TODO set defaults
type CraeftConfig struct {
	ProjectName     string        `yaml:"project_name" default:"Default Project"`
	OutputFormats   []string      `yaml:"output_formats"`
	SourceDirectory string        `yaml:"source_directory"`
	OutputDirectory string        `yaml:"output_directory"`
	Ignore          []string      `yaml:"ignore_files"`
	Server          ServerConfig  `yaml:"server"`
	Publish         PublishConfig `yaml:"publish"`
}

// TODO refactor to return config and no use global config
func loadConfig(configFile string) (*CraeftConfig, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config CraeftConfig

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	if err := defaults.Set(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
