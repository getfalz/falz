package project

import (
	"fmt"
	"os"
	"path/filepath"
)

func FindFile(basePath, targetPath, fileName string) (string, error) {
	currentPath := targetPath

	for {
		filePath := filepath.Join(currentPath, fileName)

		if _, err := os.Stat(filePath); err == nil {
			return filePath, nil
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

	return "", fmt.Errorf("%s not found in the directory hierarchy from %s up to %s", fileName, targetPath, basePath)
}
