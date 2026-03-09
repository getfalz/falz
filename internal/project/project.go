package project

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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

	FS      *FSRegistry
	Layouts LayoutRegistry

	graph DependencyGraph

	Pipes *pipeline.Registry

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

	pipes := pipeline.NewRegistry(&pipeline.RegistryConfig{
		Pipelines:       config.Pipes,
		PipelinesPath:   config.PipesPath,
		PipeTimeout:     time.Duration(config.Build.PipeTimeout) * time.Second,
		SourceDirectory: srcAbs,
		PoolSize:        config.Build.Concurrency,
	})
	if err := pipes.LoadAll(); err != nil {
		fs.Close()
		return nil, err
	}

	layouts := NewLayoutCache(fs.scheme, pipes, config.Layout)

	graph := NewDependencyGraph()

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

	return &Project{
		Config:  config,
		Pipes:   pipes,
		Layouts: layouts,
		FS:      fs,

		graph: graph,

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

func (p *Project) Build(ctx context.Context) error {
	log.IInfo(log.Project).Msg("starting build")

	source := NewSourceIndex(p.FS, p.Layouts, IgnoreList(p.Config.Ignore))

	if err := source.Scan(ctx, p.graph); err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	entries := p.graph.AllEntries()

	log.IInfo(log.Project).
		Int("entries", len(entries)).
		Msg("scan complete")

	hasher := NewHashIndex(p.FS, p.Layouts, p.Pipes)
	hashed, err := hasher.HashAll(ctx, p.graph)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	log.IInfo(log.Project).
		Int("hashed", hashed).
		Msg("hash complete")

	dirty := p.MarkDirty()
	log.IInfo(log.Project).
		Int("dirty", dirty).
		Msg("diff complete")

	if p.Config.Build.CleanStale {
		currentIDs := make(map[string]struct{})
		for _, entry := range entries {
			currentIDs[entry.ID] = struct{}{}
		}
		removed := p.Manifest.CleanStale(currentIDs)

		for _, manifestEntry := range removed {
			if err := p.FS.Output().Remove(manifestEntry.OutputPath); err != nil {
				log.Warn(fmt.Sprintf("remove stale %s: %v", manifestEntry.OutputPath, err))
			}
		}

		if len(removed) > 0 {
			log.IInfo(log.Project).
				Int("removed", len(removed)).
				Msg("cleaned stale entries")
		}

	}

	if dirty == 0 {
		log.IInfo(log.Project).Msg("nothing to build")
		return nil
	}

	dirtyEntries := p.graph.DirtyEntries()
	if p.Config.Build.ForceRebuild {
		dirtyEntries = entries
	}

	builder := NewBuilder(
		p.FS,
		p.Layouts,
		p.Pipes,
		p.Manifest,
		p.CAS,

		&BuilderConfig{
			Concurrency:  p.Config.Build.Concurrency,
			UseCAS:       p.Config.Build.UseCAS,
			ForceRebuild: p.Config.Build.ForceRebuild,
			Variables:    p.Config.Variables,
		},
	)
	// TODO report stats
	_, err = builder.Build(ctx, dirtyEntries)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	manifestPath := filepath.Join(p.Config.OutputPath, manifestoFile)
	if err := p.Manifest.Save(manifestPath); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}

	log.IInfo(log.Project).Msg("build complete")
	return nil
}

func (p *Project) Graph() DependencyGraph {
	return p.graph
}

func (p *Project) IncrementalBuild(ctx context.Context) (BuildStats, error) {
    dirtyEntries := p.graph.DirtyEntries()
    if len(dirtyEntries) == 0 {
        return BuildStats{}, nil
    }
    builder := NewBuilder(p.FS, p.Layouts, p.Pipes, p.Manifest, p.CAS, &BuilderConfig{
        Concurrency: p.Config.Build.Concurrency,
        UseCAS:      p.Config.Build.UseCAS,
        Variables:   p.Config.Variables,
    })
    stats, err := builder.Build(ctx, dirtyEntries)
    if err != nil {
        return stats, err
    }
    manifestPath := filepath.Join(p.Config.OutputPath, manifestoFile)
    if saveErr := p.Manifest.Save(manifestPath); saveErr != nil {
        return stats, fmt.Errorf("save manifest: %w", saveErr)
    }
    return stats, nil
}

func (p *Project) MarkDirty() int {
	dirty := 0
	for _, e := range p.graph.AllEntries() {
		if p.Manifest.IsDirty(e.ID, e.Hash) {
			p.graph.MarkDirty(e.ID)
			dirty++
		}
	}
	return dirty
}
