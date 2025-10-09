package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"craeft/internal/log"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

const xdgConfigFolder = "craeft"
const configSchemesFolder = "schemes"
const configPipesFolder = "pipes"
const projectRootFolder = ".craeft"
const manifestoFile = ".manifest.json"

type ServerConfig struct {
	Host string `yaml:"host" default:"localhost"`
	Port int    `yaml:"port" default:"8080"`
}

type FsWatcherConfig struct {
	Enabled   bool `yaml:"enabled" default:"true"`     // Enable file system watcher
	HotReload bool `yaml:"hot_reload" default:"false"` // Enable hot-reload
}

// TODO set start/end tag for layout.parseTpl
type TemplateConfig struct {
	OpenTag  string `yaml:"type" default:"{{"`
	CloseTag string `yaml:"close_tag" default:"}}"`
}

type HooksConfig struct {
	Pre  []string `yaml:"pre_scripts"`
	Post []string `yaml:"post_scripts"`
}

// BuildConfig configures the incremental build
type BuildConfig struct {
	ForceRebuild bool `yaml:"force,omitempty" default:"false"`  // Ignore cache, rebuild everything
	Concurrency  int  `yaml:"concurrency,omitempty"`            // Number of CPUs. It defaults to the number of logical CPUs
	UseCAS       bool `yaml:"use_cas,omitempty" default:"true"` // Use content-addressable storage
	CleanStale   bool `yaml:"clean,omitempty" default:"false"`  // Remove outputs for deleted sources
}

type CraeftConfig struct {
	OutputPath string   `yaml:"output,omitempty" default:"dist"` // Output folder for build
	Ignore     []string `yaml:"ignore,omitempty"`                // Ignore files/folders in source
	Debug      bool     `yaml:"debug,omitempty" default:"false"` // should save in-between files? see main.go

	Layout string `yaml:"layout,omitempty" default:"layout.xml"` // Which files categorize as layout Can be a pattern like *.xml

	Scheme         string         `yaml:"scheme,omitempty"`
	Pipes          []string       `yaml:"pipes,omitempty" default:"[\"md\"]"` // List of enabled pipes
	TemplateConfig TemplateConfig `yaml:"template_config,omitempty"`

	SourcePath string `yaml:"-"`
	ConfigPath string `yaml:"-"`
	SchemePath string `yaml:"-"`
	PipesPath  string `yaml:"-"`

	Server    ServerConfig    `yaml:"server,omitempty"`
	FsWatcher FsWatcherConfig `yaml:"file_watcher,omitempty"`

	Variables map[string]any `yaml:"variables,omitempty"` // Key/value pair of variables available when building. Variables starting with a $ will be replaced with env vars
	Hooks     HooksConfig    `yaml:"hooks,omitempty"`

	Build BuildConfig `yaml:"build,omitempty"`
}

func LoadConfig(base string) (*CraeftConfig, error) {
	config := &CraeftConfig{}

	config.ConfigPath = filepath.Join(base, projectRootFolder)

	if err := defaults.Set(config); err != nil {
		return nil, err
	}

	configFile := filepath.Join(config.ConfigPath, "config.yaml")

	file, err := os.Open(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn("config file not found. Using defaults.")
			return config, nil
		} else {
			return nil, err
		}
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	config.Variables = resolveEnvVars(config.Variables).(map[string]any)

	if config.Build.Concurrency <= 0 {
		config.Build.Concurrency = runtime.NumCPU()
	}

	return config, nil
}

func (c *CraeftConfig) SetPaths() {
	_, err := os.Stat(filepath.Join(c.ConfigPath, configSchemesFolder))
	if err == nil || !os.IsNotExist(err) {
		c.SchemePath = filepath.Join(c.ConfigPath, configSchemesFolder)
	} else {
		log.Warn(fmt.Sprintf("Schemes folder %q doesn't exist", filepath.Join(c.ConfigPath, configPipesFolder)))
		// c.SchemePath, err = xdg.ConfigFile(filepath.Join(xdgConfigFolder, configSchemesFolder, c.Scheme))
		// if err != nil {
		// 	log.Fatal(fmt.Sprintf("config file has no 'scheme' defined: %s", err))
		// }
	}

	// TODO: at some point, pipes can be loaded from .craeft/pipes and .config/craeft/pipes
	_, err = os.Stat(filepath.Join(c.ConfigPath, configPipesFolder))
	if err == nil || !os.IsNotExist(err) {
		c.PipesPath = filepath.Join(c.ConfigPath, configPipesFolder)
	} else {
		log.Warn(fmt.Sprintf("Pipes folder %q doesn't exist", filepath.Join(c.ConfigPath, configPipesFolder)))
		// c.PipesPath, err = xdg.ConfigFile(filepath.Join(xdgConfigFolder, configPipesFolder))
		// if err != nil {
		// 	log.Fatal(err.Error())
		// }
	}

}

func (c *CraeftConfig) IsValid() error {
	// if c.Scheme == "" {
	// 	return errors.New("no theme specified in config file")
	// }

	if c.FsWatcher.HotReload && !c.FsWatcher.Enabled {
		return errors.New("hot-reload enabled, but no file watcher enabled")
	}

	return nil
}

func resolveEnvVars(vars any) any {
	switch v := vars.(type) {
	case map[string]any:
		for key, value := range v {
			v[key] = resolveEnvVars(value)
		}
	case []any:
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
