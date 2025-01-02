package content

import (
	"craeft/internal/pipeline"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
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
		log.Fatalf("Failed to read asset file: %s", err)
	}

	vars := make(map[string]string)
	outputFile := strings.TrimSuffix(a.path, a.pipeline.Config().SourceSuffix) + a.pipeline.Config().TargetSuffix
	vars["target"] = outputFile

	transformed := a.pipeline.Transform(string(file), nil)

	var results []ContentResult
	for _, r := range transformed {
		results = append(results, ContentResult{Filename: r["filename"], Result: r["result"]})
	}

	return results, nil
}
