package project

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	Ignore *IgnoreList

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

	outAbs, _ := filepath.Abs(config.OutputPath)

	outputBase := filepath.Base(outAbs)
	ignoreBase := []string{
		filepath.ToSlash(filepath.Join(outputBase, "**")),
	}
	ignoreBase = append(ignoreBase, blackList...)

	pipes, err := pipeline.NewRegistry(&pipeline.RegistryConfig{
		Pipes:      config.Pipes,
		PipesPath:  config.PipesPath,
		SourceRoot: sourceRoot,
	})

	if err != nil {
		_ = sourceRoot.Close()
		_ = schemeRoot.Close()
		_ = outputRoot.Close()
		return nil, err
	}
	if err := pipes.LoadAll(); err != nil {
		_ = sourceRoot.Close()
		_ = schemeRoot.Close()
		_ = outputRoot.Close()
		return nil, err
	}

	layouts := NewLayoutCache(schemeRoot, pipes, config.Layout)

	graph := NewDependencyGraph()

	manifest, err := OpenManifest(outputRoot, manifestoFile)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}

	var cas *CAS
	if config.Build.UseCAS {
		cas, err = NewCAS(config.Build.CASPath, outputRoot)
		if err != nil {
			return nil, fmt.Errorf("create CAS: %w", err)
		}
	}

	ignore := NewIgnoreList(ignoreBase, config.Ignore)

	return &Project{
		Config:        config,
		PipesRegistry: pipes,
		Layouts:       layouts,

		Scheme: schemeRoot,
		Source: sourceRoot,
		Output: outputRoot,

		Ignore: ignore,

		graph: graph,

		CAS:      cas,
		Manifest: manifest,
	}, nil
}

func (p *Project) Build(ctx context.Context) error {
	log.IInfo(log.Project).Msg("starting build")

	source := NewSourceIndex(p.Source, p.Scheme, p.Layouts, p.Ignore)

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

	var dirty int
	if p.Config.Build.ForceRebuild {
		p.graph.MarkAllDirty()
		dirty = len(entries)
	} else {
		dirty = p.MarkDirty()
	}
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

	log.IInfo(log.Project).
		Int("total", stats.Scheduled).
		Int64("built", stats.Written).
		Int64("cached", stats.CacheHits).
		Dur("duration", stats.Duration).
		Msg("build complete")

	p.graph.ClearDirty()

	if manErr := p.Manifest.Save(); manErr != nil {
		return fmt.Errorf("save manifest: %w", manErr)
	}

	if err != nil {
		return fmt.Errorf("build: %w", err)
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

	p.graph.ClearDirty()

	if saveErr := p.Manifest.Save(); saveErr != nil {
		return stats, fmt.Errorf("save manifest: %w", saveErr)
	}
	return stats, nil
}

func (p *Project) ForceBuild(ctx context.Context, path string) (BuildStats, error) {
	graph := p.Graph()

	entry, ok := graph.EntryByID(path)
	if !ok {
		return BuildStats{}, fmt.Errorf("entry not found: %s", path)
	}

	if err := graph.MarkDirty(entry.ID); err != nil {
		return BuildStats{}, fmt.Errorf("mark dirty: %w", err)
	}

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
			Concurrency:  p.Config.Build.Concurrency,
			UseCAS:       p.Config.Build.UseCAS,
			ForceRebuild: true,
			Variables:    p.Config.Variables,
		})

	stats, err := builder.Build(ctx, dirtyEntries)
	if err != nil {
		return stats, err
	}

	p.graph.ClearDirty()

	if saveErr := p.Manifest.Save(); saveErr != nil {
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

// CleanCache removes tracked build outputs, the CAS store, and the build
// manifest.
func CleanCache(config *CraeftConfig) error {
	var errs []error

	outputRoot, err := utils.NewRootFs(config.OutputPath)
	if err != nil {
		return fmt.Errorf("clean: open output root: %w", err)
	}
	defer outputRoot.Close()

	manifest, err := OpenManifest(outputRoot, manifestoFile)
	if err != nil {
		return fmt.Errorf("clean: open manifest: %w", err)
	}

	removed := manifest.CleanStale(map[string]struct{}{})
	for _, entry := range removed {
		if rErr := outputRoot.Remove(entry.OutputPath); rErr != nil && !os.IsNotExist(rErr) {
			errs = append(errs, fmt.Errorf("clean output %s: %w", entry.OutputPath, rErr))
		}
	}

	if err := outputRoot.Remove(manifestoFile); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("clean manifest: %w", err))
	}

	if casPath := resolveCASPath(config.SourcePath); casPath != "" {
		casParent, err := utils.NewRootFs(filepath.Dir(casPath))
		if err != nil {
			return fmt.Errorf("clean: open CAS parent: %w", err)
		}
		defer casParent.Close()

		if err := casParent.RemoveAll(filepath.Base(casPath)); err != nil {
			return fmt.Errorf("clean CAS: %w", err)
		}
	}

	return errors.Join(errs...)
}
