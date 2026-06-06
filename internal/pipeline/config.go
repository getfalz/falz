package pipeline

import (
	"craeft/internal/pipeline/js"
	"fmt"
	"os"
	"time"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

type duration time.Duration

func (d *duration) UnmarshalYAML(value *yaml.Node) error {
	if parsed, err := time.ParseDuration(value.Value); err == nil {
		*d = duration(parsed)
		return nil
	}
	var ms int64
	if err := value.Decode(&ms); err != nil {
		return fmt.Errorf("expected a duration string (\"30s\") or milliseconds integer, got %q", value.Value)
	}
	*d = duration(time.Duration(ms) * time.Millisecond)
	return nil
}

// FetchConfig controls outbound HTTP behaviour for a JS pipeline.
type FetchConfig struct {
	// Timeout is the per-request wall-clock limit.
	// Default: 30s. Negative: no client-level deadline.
	Timeout duration `yaml:"timeout"`

	// MaxBodyBytes caps each individual response body.
	// Default: 33 554 432 (32 MiB). Negative: no cap.
	MaxBodyBytes int64 `yaml:"maxBodyBytes" default:"33554432"`

	// MaxConnsPerHost caps total connections (idle + active) to any one host
	// across all concurrent VMs in this pipeline.
	// Default: 8. Negative: no per-host cap.
	MaxConnsPerHost int `yaml:"maxConnsPerHost" default:"8"`

	// MaxRedirects caps the redirect chain per request.
	// Default: 10. Negative: no limit.
	MaxRedirects int `yaml:"maxRedirects" default:"10"`

	// MaxBytesPerTransform caps total bytes read across all http.* calls in
	// a single transform call. 0: no cumulative cap.
	MaxBytesPerTransform int64 `yaml:"maxBytesPerTransform"`

	// AllowedHosts restricts which hosts this pipeline may contact.
	// Entries: "api.example.com", "api.example.com:8080", "*.example.com".
	//
	// Required for any HTTP access. An empty list disables all outbound HTTP.
	AllowedHosts []string `yaml:"allowedHosts"`
}

func (c *FetchConfig) SetDefaults() {
	if c.Timeout == 0 {
		c.Timeout = duration(30 * time.Second)
	}
}

func (c FetchConfig) Policy() js.HTTPPolicy {
	return js.HTTPPolicy{
		Timeout:              time.Duration(c.Timeout),
		AllowedHosts:         c.AllowedHosts,
		MaxRedirects:         c.MaxRedirects,
		MaxBodyBytes:         c.MaxBodyBytes,
		MaxBytesPerTransform: c.MaxBytesPerTransform,
	}
}

// PipelineConfig is the schema for a JS pipeline's config.yaml.
type PipelineConfig struct {
	// PipelineName is the unique identifier for this pipe.
	PipelineName string `yaml:"name"`

	// EntryPoint is the TypeScript file esbuild uses as its entry point.
	EntryPoint string `yaml:"entryPoint" default:"index.ts"`

	// Debug emits an unminified pipeline.dbg.js alongside the bundle
	// for human inspection. Never enable in production builds.
	Debug bool `yaml:"debug" default:"false"`

	TransformTimeoutMs int `yaml:"transformTimeoutMs"`

	// Fetch configures the outbound HTTP client for fetch() calls
	Fetch FetchConfig `yaml:"http"`
}

func loadConfig(configFile string) (*PipelineConfig, error) {
	config := &PipelineConfig{}

	file, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if err := defaults.Set(config); err != nil {
		return nil, err
	}

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	return config, nil
}
