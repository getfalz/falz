package project

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"

	"craeft/internal/log"
	"craeft/internal/pipeline"
	"craeft/internal/utils"

	"golang.org/x/sync/errgroup"
)

// HashIndex computes and updates hashes for all entries in the dependency graph.
type HashIndex struct {
	fs      *FSRegistry
	layouts LayoutRegistry
	pipes   *pipeline.Registry
}

func NewHashIndex(
	fs *FSRegistry,
	layouts LayoutRegistry,
	pipes *pipeline.Registry,
) *HashIndex {
	return &HashIndex{
		fs:      fs,
		layouts: layouts,
		pipes:   pipes,
	}
}

func (hi *HashIndex) HashAll(ctx context.Context, graph DependencyGraph) (int, error) {
	entries := graph.AllEntries()
	if len(entries) == 0 {
		return 0, nil
	}

	// TODO use config
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(workers)
	var hashed atomic.Int64

	errs := make([]error, len(entries))

	for i, entry := range entries {
		i, entry := i, entry

		eg.Go(func() error {
			if egCtx.Err() != nil {
				return nil
			}

			hash, err := hi.ComputeHash(egCtx, entry)
			if err != nil {
				errs[i] = fmt.Errorf("hash %s: %w", entry.ID, err)

				return nil
			}

			if err := graph.UpdateEntry(entry.ID, func(e *BuildEntry) error {
				e.Hash = hash
				e.Error = nil
				return nil
			}); err != nil {
				errs[i] = fmt.Errorf("update entry %s: %w", entry.ID, err)
				return nil
			}

			hashed.Add(1)
			return nil
		})
	}

	eg.Wait()

	if ctx.Err() != nil {
		errs = append(errs, ctx.Err())
	}

	n := int(hashed.Load())
	log.IInfo(log.Plan).
		Int("hashed", n).
		Int("total", len(entries)).
		Msg("hashing complete")

	return n, errors.Join(errs...)
}

// ComputeHash computes the combined hash for a single entry, including content, layout dependencies, and pipeline configuration.
func (hi *HashIndex) ComputeHash(ctx context.Context, entry *BuildEntry) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	contentHash, err := hi.fs.Get(entry.SourceRoot).HashFile(entry.SourcePath)
	if err != nil {
		return "", fmt.Errorf("content: %w", err)
	}

	var layoutHash string
	if entry.Type == ContentType && entry.LayoutKey.LayoutPath != "" {
		layoutHash, err = hi.hashLayoutDeps(ctx, entry)
		if err != nil {
			return "", fmt.Errorf("layout deps: %w", err)
		}
	}

	pipelineHash := hi.hashPipeline(entry.Pipeline)

	return utils.CombineHashes(
		contentHash,
		layoutHash,
		pipelineHash,
	), nil
}

func (hi *HashIndex) hashLayoutDeps(ctx context.Context, entry *BuildEntry) (string, error) {
	layoutHash, err := hi.fs.Scheme().HashFile(entry.LayoutKey.LayoutPath)
	if err != nil {
		return "", fmt.Errorf("layout %s: %w", entry.LayoutKey.LayoutPath, err)
	}

	lyt, err := hi.layouts.Get(ctx, entry.LayoutKey)
	if err != nil {
		// Layout not yet compiled (e.g. first build) - not a fatal error.
		log.DDebug(log.Plan).
			Str("layout", entry.LayoutKey.LayoutPath).
			Str("context", entry.LayoutKey.CtxPath).
			Msg("layout deps not cached, skipping fragment hashes")
		return "", nil
	}

	fragmentHashes := make([]string, len(lyt.GetDeps()))
	for _, path := range lyt.GetDeps() {
		fragHash, err := hi.fs.Scheme().HashFile(path)
		if err != nil {
			return "", fmt.Errorf("fragment %s: %w", path, err)
		}
		fragmentHashes = append(fragmentHashes, fragHash)
	}

	return utils.CombineHashes(append(fragmentHashes, layoutHash)...), nil
}

func (hi *HashIndex) hashPipeline(name string) string {
	if name == "" {
		return ""
	}
	pl, err := hi.pipes.Get(name)
	if err != nil {
		return ""
	}
	cfg := pl.Config()
	return utils.HashString(fmt.Sprintf("%s-%s-%s",
		cfg.PipelineName, cfg.SourceSuffix, cfg.TargetSuffix))
}
