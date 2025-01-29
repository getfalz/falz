package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattn/go-zglob"
	log "github.com/sirupsen/logrus"
)

// FindFile searches a file fileName by consuming the targetPath and going up the directory hierarchy until reaching the basePath. Then it returns a map of fileName to the relative path of the file with the basePath.
func FindFile(basePath, targetPath, pattern string) ([]string, error) {
	var results []string
	uniqueResult := make(map[string]bool)
	currentPath := filepath.Join(basePath, targetPath)

	for {
		matches, err := zglob.Glob(filepath.Join(currentPath, pattern))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("findFile can't glob '%s' in '%s': %w", pattern, currentPath, err)
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
		return nil, fmt.Errorf("no files matching pattern %s found from %s up to %s", pattern, targetPath, basePath)
	}

	return results, nil
}

// NormalizePath returns an absolute path relative to the given basePath
func NormalizePath(basePath string, path string) (string, error) {
	log.Infof("normalizing path %s with base path %s", path, basePath)
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
