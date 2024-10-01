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

type StylesConfig struct {
	Parser     string `yaml:"parser"` // scss, css
	Compressed bool   `yaml:"compressed"`
	SourceMap  bool   `yaml:"sourcemap"`
	SassPath   string `yaml:"sass_path" default:"sass"`
}

type ScriptsConfig struct {
	Parser     string `yaml:"parser"`
	Compressed bool   `yaml:"compressed"`
	SourceMap  bool   `yaml:"sourcemap"`
}

// TODO set defaults
type CraeftConfig struct {
	ProjectName     string `yaml:"project_name" default:"Default Project"`
	SourceDirectory string
	OutputDirectory string        `yaml:"output_directory"`
	Ignore          []string      `yaml:"ignore_files"`
	Debug           bool          `yaml:"debug"`
	EnableHotReload bool          `yaml:"enable_hot_reload"`
	ThemePath       string        `yaml:"theme"`
	StylesConfig    StylesConfig  `yaml:"styles"`
	ScriptsConfig   ScriptsConfig `yaml:"scripts"`

	OutputFormats []string      `yaml:"output_formats"`
	Server        ServerConfig  `yaml:"server"`
	Publish       PublishConfig `yaml:"publish"`
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
