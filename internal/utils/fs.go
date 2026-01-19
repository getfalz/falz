package utils

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"craeft/internal/log"
)

func NormRel(p string) string {
	// Your matches from fs.Glob are slash-separated already, but normalize anyway.
	p = strings.ReplaceAll(p, "\\", "/") // just in case
	p = path.Clean(p)
	if p == "." {
		return "."
	}
	return strings.TrimPrefix(p, "./")
}

// NormalizePath returns an absolute path relative to the given basePath
func NormalizePath(basePath string, path string) (string, error) {
	clean := filepath.Clean(path)

	if filepath.IsAbs(clean) {
		rel, err := filepath.Rel(basePath, clean)
		if err != nil {
			return "", err
		}
		return "/" + filepath.ToSlash(rel), nil
	}

	abs := filepath.Join(basePath, clean)

	rel, err := filepath.Rel(basePath, abs)
	if err != nil {
		return "", err
	}
	return "/" + filepath.ToSlash(rel), nil
}

func StopComponent(ctx context.Context, name string, stop func(context.Context) error) error {
	var lg = log.Logger().With().Str("phase", "shutdown").Str("component", name).Logger()

	begin := time.Now()

	lg.Debug().Msg("stopping")
	err := stop(ctx)
	dur := time.Since(begin)

	if err != nil {
		lg.Warn().Dur("dur", dur).Err(err).Msg("failed")
		return fmt.Errorf("stop %s: %w", name, err)
	}
	lg.Debug().Dur("dur", dur).Msg("stopped")
	return nil
}
