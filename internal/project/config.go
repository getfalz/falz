package project

import (
	"os"
	"path/filepath"

	"craeft/internal/log"

	"github.com/adrg/xdg"
	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

const configFolder = "craeft"

type ServerConfig struct {
	Host string `yaml:"host" default:"localhost"`
	Port int    `yaml:"port" default:"8080"`
}

type FsWatcherConfig struct {
	HotReload bool `yaml:"hot_reload" default:"false"`
	Content   bool `yaml:"content" default:"false"`
	Theme     bool `yaml:"theme" default:"false"`
}

// TODO set start/end tag for layout.parseTpl
type TemplateConfig struct {
	Type string `yaml:"type" default:"simple"`
}

type HooksConfig struct {
	Pre  []string `yaml:"pre_scripts"`
	Post []string `yaml:"post_scripts"`
}

type CraeftConfig struct {
	ProjectName     string `yaml:"project_name" default:"Default Project"`
	SourceDirectory string
	OutputDirectory string   `yaml:"output" default:"dist"`
	Ignore          []string `yaml:"ignore"`
	Debug           bool     `yaml:"debug" default:"false"` // should save in-between files? see main.go

	Layout string `yaml:"layout" default:"layout.xml"`

	Theme          string         `yaml:"theme"`
	Pipelines      []string       `yaml:"pipelines" default:"[\"md\"]"`
	TemplateConfig TemplateConfig `yaml:"template_config"`

	ConfigBase    string
	ThemePath     string
	PipelinesPath string

	Server    ServerConfig    `yaml:"server"`
	FsWatcher FsWatcherConfig `yaml:"file_watcher"`

	Variables map[string]any `yaml:"variables"`
	Hooks     HooksConfig    `yaml:"hooks"`
}

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

	config.Variables = resolveEnvVars(config.Variables).(map[string]any)

	if err := defaults.Set(&config); err != nil {
		return nil, err
	}
	config.ConfigBase, err = xdg.ConfigFile(filepath.Join(configFolder))
	if err != nil {
		log.Fatal(err.Error())
	}
	config.ThemePath, err = xdg.ConfigFile(filepath.Join(configFolder, "themes", config.Theme))
	if err != nil {
		log.Fatal(err.Error())
	}

	config.PipelinesPath, err = xdg.ConfigFile(filepath.Join(configFolder, "pipelines"))
	if err != nil {
		log.Fatal(err.Error())
	}

	if config.Theme == "" {
		log.Fatal("No theme specified in config file")
	}

	if config.FsWatcher.HotReload && (!config.FsWatcher.Content || !config.FsWatcher.Theme) {
		log.Warn("Hot-reload enabled, but no file watcher enabled")
	}

	return &config, nil
}

func resolveEnvVars(vars any) any {
	switch v := vars.(type) {
	case map[string]interface{}:
		for key, value := range v {
			v[key] = resolveEnvVars(value)
		}
	case []interface{}:
		for i, item := range v {
			v[i] = resolveEnvVars(item)
		}
	case string:
		if len(v) > 1 && v[0] == '$' {
			envVar := v[1:]
			if val, exists := os.LookupEnv(envVar); exists {
				return val
			}
			return ""
		}
	}
	return vars
}
