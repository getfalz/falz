package utils

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"craeft/internal/log"
)

// FindFile searches for files matching the given glob pattern by walking up the directory
// tree, starting from the combined basePath and targetPath, and stopping at basePath.
// It returns a slice of relative file paths (relative to basePath) for each unique match found.
//
// For example, if pattern is "*.config" and targetPath is "src/app", the function will look for
// matching files in:
//
//	basePath/src/app
//	basePath/src
//	basePath
//
// The search stops once basePath is reached. If no matches are found, an error is returned.
func FindFile(root fs.FS, targetPath, pattern string) ([]string, error) {
	cur := filepath.ToSlash(strings.TrimPrefix(targetPath, "/"))
	if cur == "" {
		cur = "."
	}

	var results []string
	seen := make(map[string]struct{})
	for {
		glob := pattern
		if cur != "." {
			glob = path.Join(cur, pattern)
		}

		matches, err := fs.Glob(root, glob)
		if err != nil {
			// fs.Glob only errors on malformed patterns
			return nil, fmt.Errorf("glob %q: %w", glob, err)
		}

		for _, fileName := range matches {
			if _, ok := seen[fileName]; !ok {
				results = append(results, fileName)
				seen[fileName] = struct{}{}
			}
		}

		if cur == "." {
			break
		}

		next := path.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no files matching %q found while walking up from %q", pattern, targetPath)
	}

	return results, nil
}

type mkdirer interface {
	Mkdir(name string, perm os.FileMode) error
}

func MkdirAllFS(fs mkdirer, p string, perm os.FileMode) error {
	p = filepath.Clean(p)
	if p == "." {
		return nil
	}

	cur := filepath.VolumeName(p)
	if filepath.IsAbs(p) {
		cur += string(filepath.Separator)
		p = strings.TrimPrefix(p, cur)
	}

	for part := range strings.SplitSeq(p, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		if err := fs.Mkdir(cur, perm); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	return nil
}

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
