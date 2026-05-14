package project

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

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

	Source *utils.RootFS
	Scheme *utils.RootFS
	Output *utils.RootFS

	Layouts *LayoutCache

	graph DependencyGraph

	PipesRegistry *pipeline.Registry

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

	// TODO do we need absolute paths here?
	outAbs, _ := filepath.Abs(config.OutputPath)
	srcAbs, _ := filepath.Abs(config.SourcePath)

	fmt.Printf("output abs: %s, srcAbs: %s\n", outAbs, srcAbs)
	outputBase := filepath.Base(outAbs)
	config.Ignore = append(config.Ignore,
		filepath.ToSlash(filepath.Join(outputBase, "**")),
	)
	config.Ignore = append(config.Ignore, blackList...)

	pipes := pipeline.NewRegistry(&pipeline.RegistryConfig{
		Pipes:      config.Pipes,
		PipesPath:  config.PipesPath,
		SourceRoot: sourceRoot,
	})
	if err := pipes.LoadAll(); err != nil {
		_ = sourceRoot.Close()
		_ = schemeRoot.Close()
		_ = outputRoot.Close()
		return nil, err
	}

	layouts := NewLayoutCache(schemeRoot, pipes, config.Layout)

	graph := NewDependencyGraph()

	manifest, err := LoadManifest(filepath.Join(config.OutputPath, manifestoFile))
	if err != nil {
		log.Warn(fmt.Sprintf("failed to load manifest: %v. Creating a new one", err))
		manifest = NewManifest()
	}

	var cas *CAS
	if config.Build.UseCAS {
		cas, err = NewCAS(config.Build.CASPath, outputRoot)
		if err != nil {
			return nil, fmt.Errorf("create CAS: %w", err)
		}
	}

	return &Project{
		Config:        config,
		PipesRegistry: pipes,
		Layouts:       layouts,

		Scheme: schemeRoot,
		Source: sourceRoot,
		Output: outputRoot,

		graph: graph,

		CAS:      cas,
		Manifest: manifest,
	}, nil
}

func (p *Project) Build(ctx context.Context) error {
	log.IInfo(log.Project).Msg("starting build")

	p.graph.Clear()

	source := NewSourceIndex(p.Source, p.Scheme, p.Layouts, IgnoreList(p.Config.Ignore))

	if err := source.Scan(ctx, p.graph); err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	entries := p.graph.AllEntries()

	log.IInfo(log.Project).
		Int("entries", len(entries)).
		Msg("scan complete")

	hasher := NewHashIndex(p.Source, p.Scheme, p.Layouts, p.PipesRegistry)
	err := hasher.HashAll(ctx, p.graph)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}

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
			if err := p.Output.Remove(manifestEntry.OutputPath); err != nil {
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
		p.Source, p.Scheme, p.Output,
		p.Layouts,
		p.PipesRegistry,
		p.Manifest,
		p.CAS,

		&BuilderConfig{
			Concurrency:  p.Config.Build.Concurrency,
			UseCAS:       p.Config.Build.UseCAS,
			ForceRebuild: p.Config.Build.ForceRebuild,
			Variables:    p.Config.Variables,
		},
	)
	stats, err := builder.Build(ctx, dirtyEntries)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	log.IInfo(log.Project).
		Int("total", stats.Scheduled).
		Int64("built", stats.Written).
		Int64("cached", stats.CacheHits).
		Dur("duration", stats.Duration).
		Msg("build complete")

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
	builder := NewBuilder(
		p.Source, p.Scheme, p.Output,
		p.Layouts,
		p.PipesRegistry,
		p.Manifest,
		p.CAS,
		&BuilderConfig{
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
			log.DDebug(log.Plan).
				Str("id", e.ID).
				Str("hash", e.Hash[:8]).
				Msg("dirty")
		}
	}
	return dirty
}

func (p *Project) Close() error {
	var err error
	if e := p.Source.Close(); e != nil {
		err = errors.Join(err, e)
	}
	if e := p.Scheme.Close(); e != nil {
		err = errors.Join(err, e)
	}
	if e := p.Output.Close(); e != nil {
		err = errors.Join(err, e)
	}
	if p.CAS != nil {
		if e := p.CAS.Close(); e != nil {
			err = errors.Join(err, e)
		}
	}
	if e := p.PipesRegistry.Close(); e != nil {
		err = errors.Join(err, e)
	}
	return err
}
