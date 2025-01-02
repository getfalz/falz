package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattn/go-zglob"
	log "github.com/sirupsen/logrus"
)

// FindFile searches a file fileName by consuming the targetPath and going up the directory hierarchy until reaching the basePath. Then it returns a map of fileName to the full path of the file.
func FindFile(basePath, targetPath, pattern string) (map[string]string, error) {
	result := make(map[string]string)
	currentPath := filepath.Join(basePath, targetPath)

	for {
		log.Infof("globing in %s", filepath.Join(currentPath, pattern))
		matches, err := zglob.Glob(filepath.Join(currentPath, pattern))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("findFile can't glob '%s' in '%s': %w", pattern, currentPath, err)
		}

		for _, match := range matches {
			fileName := filepath.Base(match)
			if _, ok := result[fileName]; !ok {
				result[fileName] = match
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

	if len(result) == 0 {
		return nil, fmt.Errorf("no files matching pattern %s found from %s up to %s", pattern, targetPath, basePath)
	}

	return result, nil
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
