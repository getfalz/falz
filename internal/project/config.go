package project

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"craeft/internal/log"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

const xdgConfigFolder = "craeft"
const configSchemesFolder = "schemes"
const configPipesFolder = "pipes"
const projectRootFolder = ".craeft"
const configFilePath = "config.yaml"
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
	ForceRebuild bool   `yaml:"force,omitempty" default:"false"`    // Ignore cache, rebuild everything
	Concurrency  int    `yaml:"concurrency,omitempty"`              // Number of CPUs. It defaults to the number of logical CPUs
	UseCAS       bool   `yaml:"use_cas,omitempty" default:"true"`   // Use content-addressable storage
	CASPath      string `yaml:"cas_path,omitempty"`                 // Path to content-addressable storage. Defaults to $XDG_CACHE_HOME/.craeft
	CleanStale   bool   `yaml:"clean,omitempty" default:"false"`    // Remove outputs for deleted sources
	PipeTimeout  int    `yaml:"pipe_timeout,omitempty" default:"3"` // Timeout for each pipe in seconds
}

type CraeftConfig struct {
	OutputPath string   `yaml:"output,omitempty" default:"dist"` // Output folder for build
	Ignore     []string `yaml:"ignore,omitempty"`                // Ignore files/folders in source
	Debug      bool     `yaml:"debug,omitempty" default:"false"` // should save in-between files? see main.go

	Layout string `yaml:"layout,omitempty" default:"layout.xml"` // Which files categorize as layout Can be a pattern like *.xml

	Scheme         string         `yaml:"scheme,omitempty"`
	Pipes          []string       `yaml:"pipes,omitempty" default:"[\"md\"]"` // List of enabled pipes
	TemplateConfig TemplateConfig `yaml:"template_config,omitempty"`

	SourcePath   string `yaml:"-"`
	ConfigFile   string `yaml:"-"`
	ConfigFolder string `yaml:"-"`
	SchemePath   string `yaml:"-"`
	PipesPath    string `yaml:"-"`

	Server    ServerConfig    `yaml:"server,omitempty"`
	FsWatcher FsWatcherConfig `yaml:"file_watcher,omitempty"`

	Variables map[string]any `yaml:"variables,omitempty"` // Key/value pair of variables available when building. Variables starting with a $ will be replaced with env vars
	Hooks     HooksConfig    `yaml:"hooks,omitempty"`

	Build BuildConfig `yaml:"build,omitempty"`
}

func LoadConfig(base string) (*CraeftConfig, error) {
	config := &CraeftConfig{}

	config.ConfigFolder = filepath.Join(base, projectRootFolder)

	if err := defaults.Set(config); err != nil {
		return nil, err
	}

	configFile := filepath.Join(config.ConfigFolder, configFilePath)

	file, err := os.Open(configFile)
	if err == nil {
		decoder := yaml.NewDecoder(file)
		if err := decoder.Decode(&config); err != nil {
			return nil, err
		}
	} else {
		if os.IsNotExist(err) {
			log.Warn("config file not found. Using defaults.")
		} else {
			return nil, err
		}
	}
	defer file.Close()

	config.ConfigFile = configFile
	config.OutputPath = filepath.Join(base, config.OutputPath)
	config.SourcePath = base
	config.PipesPath = filepath.Join(config.ConfigFolder, configPipesFolder)
	config.SchemePath = filepath.Join(config.ConfigFolder, configSchemesFolder)
	config.Variables = resolveEnvVars(config.Variables).(map[string]any)

	if config.Build.Concurrency <= 0 {
		config.Build.Concurrency = runtime.NumCPU()
	}

	if config.Build.UseCAS {
		config.SetCasPath(config.Build.CASPath)
	}

	return config, nil
}

func (c *CraeftConfig) IsValid() error {

	for name, dir := range map[string]string{"scheme": c.SchemePath, "pipes": c.PipesPath} {
		_, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("%s folder %q doesn't exist", name, dir)
		}
	}

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

func (c *CraeftConfig) SetCasPath(path string) bool {

	path = strings.TrimSpace(path)
	if path == "" {
		base, err := os.UserCacheDir()
		if err != nil || base == "" {
			return false
		}
		sum := sha1.Sum([]byte(filepath.Clean(c.SourcePath)))
		projectID := hex.EncodeToString(sum[:8])
		path = filepath.Join(base, xdgConfigFolder, projectID)
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(c.SourcePath, path)
	}

	if rel, err := filepath.Rel(c.SourcePath, path); err == nil {
		if rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+(string(filepath.Separator))) {
			subdir := filepath.ToSlash(strings.TrimPrefix(rel, c.SourcePath+string(filepath.Separator)))
			c.Ignore = append(c.Ignore, subdir+"/**")
		}
	}
	c.Build.CASPath = path

	return true
}
