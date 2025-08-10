package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"craeft/internal/log"

	"github.com/mattn/go-zglob"
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
func FindFile(basePath, targetPath, pattern string) ([]string, error) {
	var results []string
	uniqueResult := make(map[string]bool)
	currentPath := filepath.Join(basePath, targetPath)

	for {
		matches, err := zglob.Glob(filepath.Join(currentPath, pattern))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to glob pattern %q in directory %q: %w", pattern, currentPath, err)
		}

		for _, match := range matches {
			fileName, _ := filepath.Rel(basePath, match)
			if _, ok := uniqueResult[fileName]; !ok {
				results = append(results, fileName)
				uniqueResult[fileName] = true
			}
		}

		if currentPath == basePath {
			break
		}

		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			break
		}
		currentPath = parentPath
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no files matching pattern %q found while walking from %q up to %q", pattern, filepath.Join(basePath, targetPath), basePath)
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
