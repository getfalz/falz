package content

import (
	"fmt"
	"os"
	"path/filepath"

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

	log.Trace(fmt.Sprintf("New asset %s with pipeline %v", fileName, pl.Config().PipelineName))
	target := pl.Config().SourceSuffixRe.ReplaceAllString(fileName, "."+pl.Config().TargetSuffix)

	vars := pipeline.NewVars(target)

	return &Asset{
		path:     path,
		pipeline: pl,
		vars:     vars,
	}
}

func (a *Asset) Build() ([]ContentResult, error) {

	var results []ContentResult

	file, err := os.ReadFile(a.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read asset file: %s", err)
	}

	log.Debug(fmt.Sprintf("Transforming asset: %s with %+v", a.path, a.pipeline.Config().PipelineName))
	transformed := a.pipeline.Transform(file, a.vars)

	for _, r := range transformed {
		results = append(results, ContentResult{Filename: r["filename"], Result: r["result"]})
	}

	return results, nil
}
