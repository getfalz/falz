package project

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"

	"craeft/internal/log"
	"craeft/internal/utils"

	"golang.org/x/sync/errgroup"
)

type fileHasher interface {
	HashFile(path string) (string, error)
}

type layoutLoader interface {
	Get(ctx context.Context, key LayoutKey, contentVars map[string]string) (*LayoutCacheEntry, error)
}

type entryHashGraph interface {
	AllEntries() []*BuildEntry
	UpdateEntry(id string, fn func(*BuildEntry) error) error
}

// HashIndex computes and updates hashes for all entries in the dependency graph.
type HashIndex struct {
	sourceFS fileHasher
	schemeFS fileHasher

	layouts layoutLoader
	pipes   pipelineLoader
}

func NewHashIndex(
	sourceFS fileHasher,
	schemeFS fileHasher,
	layouts layoutLoader,
	pipes pipelineLoader,
) *HashIndex {
	return &HashIndex{
		sourceFS: sourceFS,
		schemeFS: schemeFS,
		layouts:  layouts,
		pipes:    pipes,
	}
}

func (hi *HashIndex) HashAll(ctx context.Context, graph DependencyGraph) error {
	entries := graph.AllEntries()
	if len(entries) == 0 {
		return nil
	}

	// TODO use config
	workers := max(runtime.NumCPU(), 1)

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(workers)
	var hashed atomic.Int64

	errs := make([]error, len(entries))

	for i, entry := range entries {
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

	return errors.Join(errs...)
}

func (hi *HashIndex) hasherFS(root SourceType) (fileHasher, error) {
	switch root {
	case SourceTypeSource:
		return hi.sourceFS, nil
	case SourceTypeScheme:
		return hi.schemeFS, nil
	default:
		return nil, fmt.Errorf("unknown source root: %v", root)
	}
}

// ComputeHash computes the combined hash for a single entry, including content, layout dependencies, and pipeline configuration.
func (hi *HashIndex) ComputeHash(ctx context.Context, entry *BuildEntry) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	fs, err := hi.hasherFS(entry.SourceRoot)
	if err != nil {
		return "", fmt.Errorf("root fs: %w", err)
	}
	contentHash, err := fs.HashFile(entry.SourcePath)
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
	layoutHash, err := hi.schemeFS.HashFile(entry.LayoutKey.LayoutPath)
	if err != nil {
		return "", fmt.Errorf("layout %s: %w", entry.LayoutKey.LayoutPath, err)
	}

	lyt, err := hi.layouts.Get(ctx, entry.LayoutKey, entry.ContentVars)
	if err != nil {
		return "", err
	}

	fragmentHashes := make([]string, 0, len(lyt.GetDeps())+len(lyt.Layout().Fragments()))
	for _, path := range lyt.GetDeps() {
		fragHash, err := hi.schemeFS.HashFile(path)
		if err != nil {
			return "", fmt.Errorf("fragment %s: %w", path, err)
		}
		fragmentHashes = append(fragmentHashes, fragHash)
	}
	for _, frag := range lyt.Layout().Fragments() {
		fragmentHashes = append(fragmentHashes, hi.hashPipeline(frag.Pipe))
	}

	return utils.CombineHashes(append(fragmentHashes, layoutHash)...), nil
}

func (hi *HashIndex) hashPipeline(name string) string {
	pl, err := hi.pipes.Get(name)
	if err != nil {
		return ""
	}
	return pl.Hash()
}
