package content

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattn/go-zglob"

	"craeft/internal/log"
	"craeft/internal/pipeline"
)

type Asset struct {
	path     string
	pipeline pipeline.PipelineScript
	vars     pipeline.Vars
}

func NewAsset(path string, pl pipeline.PipelineScript) *Asset {
	fileName := filepath.Base(path)

	log.Trace(fmt.Sprintf("New asset %s with pipeline %v", fileName, pl))
	target := pl.Config().SourceSuffixRe.ReplaceAllString(fileName, "."+pl.Config().TargetSuffix)

	vars := pipeline.NewVars(target)

	return &Asset{
		path:     path,
		pipeline: pl,
		vars:     vars,
	}
}

func (a *Asset) Build() ([]ContentResult, error) {
	matches, err := zglob.Glob(a.path)
	if err != nil {
		return nil, fmt.Errorf("failed building asset with path %s: %v", a.path, err)
	}

	var results []ContentResult
	for _, match := range matches {
		file, err := os.ReadFile(match)
		if err != nil {
			log.Error(fmt.Errorf("failed to read asset file: %s", err))
			continue
		}

		log.Debug(fmt.Sprintf("Transforming asset: %s with %+v", match, a.pipeline))
		transformed := a.pipeline.Transform(file, a.vars)

		for _, r := range transformed {
			results = append(results, ContentResult{Filename: r["filename"], Result: r["result"]})
		}
	}

	return results, nil
}
