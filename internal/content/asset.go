package content

import (
	"craeft/internal/pipeline"
	"os"

	log "github.com/sirupsen/logrus"
)

type Asset struct {
	path     string
	pipeline *pipeline.Pipeline
}

func NewAsset(path string, pipeline *pipeline.Pipeline) *Asset {
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

	transformed := a.pipeline.Transform(string(file), nil)

	var results []ContentResult
	for _, r := range transformed {
		results = append(results, ContentResult{Filename: r["filename"], Result: r["result"]})
	}

	return results, nil
}
