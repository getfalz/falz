package content

import (
	"craeft/internal/log"
	"craeft/internal/pipeline"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type Asset struct {
	path     string
	pipeline pipeline.PipelineScript
}

func NewAsset(path string, pipeline pipeline.PipelineScript) *Asset {
	return &Asset{
		path:     path,
		pipeline: pipeline,
	}
}

func (a *Asset) Build() ([]ContentResult, error) {
	file, err := os.ReadFile(a.path)
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to read asset file: %s", err))
	}

	re, err := regexp.Compile(fmt.Sprintf(`\.(%s)\b`, a.pipeline.Config().SourceSuffix))
	if err != nil {
		return nil, fmt.Errorf("failed to compile source_suffix pattern: %s", err)
	}

	target := re.ReplaceAllString(filepath.Base(a.path), "."+a.pipeline.Config().TargetSuffix)
	vars := pipeline.NewVars(target)

	transformed := a.pipeline.Transform(file, vars)

	var results []ContentResult
	for _, r := range transformed {
		results = append(results, ContentResult{Filename: r["filename"], Result: r["result"]})
	}

	return results, nil
}
