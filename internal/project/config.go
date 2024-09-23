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
	Server          ServerConfig  `yaml:"server"`
	Publish         PublishConfig `yaml:"publish"`
}

var AppConfig CraeftConfig

func LoadConfig(configFile string) error {
	file, err := os.Open(configFile)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&AppConfig); err != nil {
		return err
	}

	if err := defaults.Set(&AppConfig); err != nil {
		return err
	}

	return nil
}
