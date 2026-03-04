package project

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"
)

var (
	blackList = []string{
		fmt.Sprintf("%s/**", projectRootFolder),
		"config.yaml",
		"*.dbg.*",
	}
)

type SourceType int

const (
	SourceTypeUnknown SourceType = iota
	SourceTypeSource
	SourceTypeScheme
	SourceTypeOutput
)

type Project struct {
	Config *CraeftConfig

	FS      *FSRegistry
	Layouts LayoutRegistry
	si      *SourceIndex
	hi      *HashIndex

	Pipes *pipeline.Registry

	currentPlanMu sync.Mutex
	currentPlan   *Plan

	Manifest *Manifest
	CAS      *CAS
}

// TODO move to utils.fsroot
type FSRegistry struct {
	source *utils.RootFS
	scheme *utils.RootFS
	output *utils.RootFS
}

func NewFSRegistry(source, scheme, output string) (*FSRegistry, error) {
	sourceRoot, err := utils.NewRootFs(source)
	if err != nil {
		return nil, fmt.Errorf("source root fs: %w", err)
	}

	schemeRoot, err := utils.NewRootFs(scheme)
	if err != nil {
		_ = sourceRoot.Close()
		return nil, fmt.Errorf("scheme root fs: %w", err)
	}
	outputRoot, err := utils.NewRootFs(output)
	if err != nil {
		_ = sourceRoot.Close()
		_ = schemeRoot.Close()
		return nil, fmt.Errorf("output root fs: %w", err)
	}

	return &FSRegistry{
		source: sourceRoot,
		scheme: schemeRoot,
		output: outputRoot,
	}, nil
}
func (r *FSRegistry) Get(name SourceType) *utils.RootFS {
	switch name {
	case SourceTypeSource:
		return r.source
	case SourceTypeScheme:
		return r.scheme
	case SourceTypeOutput:
		return r.output
	default:
		return nil
	}
}

func (r *FSRegistry) Source() *utils.RootFS { return r.source }
func (r *FSRegistry) Scheme() *utils.RootFS { return r.scheme }
func (r *FSRegistry) Output() *utils.RootFS { return r.output }

func (r *FSRegistry) Close() error {
	var err error
	if e := r.source.Close(); e != nil {
		err = errors.Join(err, e)
	}
	if e := r.scheme.Close(); e != nil {
		err = errors.Join(err, e)
	}
	if e := r.output.Close(); e != nil {
		err = errors.Join(err, e)
	}

	return err
}

func New(config *CraeftConfig) (*Project, error) {
	log.IInfo(log.Project).
		Str("src", config.SourcePath).
		Str("scheme", config.SchemePath).
		Str("out", config.OutputPath).
		Msg("project init")
	if err := config.IsValid(); err != nil {
		return nil, err
	}

	fs, err := NewFSRegistry(config.SourcePath, config.SchemePath, config.OutputPath)
	if err != nil {
		return nil, err
	}

	outAbs, _ := filepath.Abs(config.OutputPath)
	srcAbs, _ := filepath.Abs(config.SourcePath)

	outputBase := filepath.Base(outAbs)
	config.Ignore = append(config.Ignore,
		filepath.ToSlash(filepath.Join(outputBase, "**")),
	)
	config.Ignore = append(config.Ignore, blackList...)

	manifest, err := LoadManifest(filepath.Join(config.OutputPath, manifestoFile))
	if err != nil {
		log.Warn(fmt.Sprintf("failed to load manifest: %v. Creating a new one", err))
		manifest = NewManifest()
	}

	var cas *CAS
	if config.Build.UseCAS {
		cas, err = NewCAS(config.Build.CASPath)
		if err != nil {
			return nil, fmt.Errorf("create CAS: %w", err)
		}
	}

	reg := pipeline.NewRegistry(&pipeline.RegistryConfig{
		Pipelines:       config.Pipes,
		PipelinesPath:   config.PipesPath,
		PipeTimeout:     time.Duration(config.Build.PipeTimeout) * time.Second,
		SourceDirectory: srcAbs,
		PoolSize:        config.Build.Concurrency,
	})
	if err := reg.LoadAll(); err != nil {
		fs.Close()
		return nil, err
	}

	layoutCache := NewLayoutCache(fs.scheme, reg, config.Layout)

	si := NewSourceIndex(fs.source, layoutCache, SourceIndexConfig{Ignore: config.Ignore})
	hi := NewHashIndex(fs, layoutCache, reg)

	return &Project{
		Config:  config,
		Pipes:   reg,
		Layouts: layoutCache,
		FS:      fs,
		si:      si,
		hi:      hi,

		CAS:      cas,
		Manifest: manifest,
	}, nil
}

func (p *Project) Close() error {
	log.IInfo(log.Project).Msg("closing roots")
	if e := p.FS.Close(); e != nil {
		return e
	}
	return nil
}
