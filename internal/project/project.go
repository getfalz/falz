package project

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
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

type Project struct {
	Config *CraeftConfig

	SourceFS *utils.RootFS
	SchemeFS *utils.RootFS
	outputFS *utils.RootFS

	Layouts LayoutRegistry

	Pipes *pipeline.Registry

	currentPlanMu sync.Mutex
	currentPlan   *Plan

	Manifest *Manifest
	CAS      *CAS
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

	sourceRoot, err := utils.NewRootFs(config.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("source root fs: %w", err)
	}

	schemeRoot, err := utils.NewRootFs(config.SchemePath)
	if err != nil {
		_ = sourceRoot.Close()
		return nil, fmt.Errorf("scheme root fs: %w", err)
	}
	outputRoot, err := utils.NewRootFs(config.OutputPath)
	if err != nil {
		_ = sourceRoot.Close()
		_ = schemeRoot.Close()
		return nil, fmt.Errorf("output root fs: %w", err)
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
		_ = sourceRoot.Close()
		_ = schemeRoot.Close()
		_ = outputRoot.Close()
		return nil, err
	}

	layoutCache := NewLayoutCache(schemeRoot, reg, config.Layout)

	return &Project{
		Config:   config,
		Pipes:    reg,
		Layouts:  layoutCache,
		SourceFS: sourceRoot,
		SchemeFS: schemeRoot,
		outputFS: outputRoot,

		CAS:      cas,
		Manifest: manifest,
	}, nil
}

func (p *Project) IgnoreFile(name string) bool {
	name = filepath.ToSlash(filepath.Clean(name))

	for _, raw := range p.Config.Ignore {
		pat := filepath.ToSlash(strings.TrimSpace(raw))
		if pat == "" {
			continue
		}

		if strings.HasSuffix(pat, "/**") {
			prefix := strings.TrimSuffix(pat, "/**")
			if name == prefix || strings.HasPrefix(name, prefix+"/") {
				log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by prefix/**")
				return true
			}
			continue
		}
		if !strings.ContainsAny(pat, "*?[]") {
			if name == pat || strings.HasPrefix(name, pat+"/") {
				log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by exact/prefix")
				return true
			}
			continue
		}

		ok, err := path.Match(pat, name)
		if err != nil {
			continue
		}
		if ok {
			log.TTrace(log.Project).Str("name", name).Str("pattern", pat).Msg("ignored by glob")
			return true
		}
	}
	return false
}

func (p *Project) CurrentPlan() (*Plan, error) {
	p.currentPlanMu.Lock()
	defer p.currentPlanMu.Unlock()

	if p.currentPlan == nil {
		return nil, fmt.Errorf("no current plan, call Plan() first")
	}
	return p.currentPlan, nil
}

func (p *Project) Close() error {
	var err error
	log.IInfo(log.Project).Msg("closing roots")
	if e := p.SourceFS.Close(); e != nil {
		err = errors.Join(err, e)
	}
	if e := p.SchemeFS.Close(); e != nil {
		err = errors.Join(err, e)
	}
	if e := p.outputFS.Close(); e != nil {
		err = errors.Join(err, e)
	}
	return err
}
