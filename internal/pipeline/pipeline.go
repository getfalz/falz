package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
	log "github.com/sirupsen/logrus"
)

type script struct {
	name   string
	path   string
	config *JSConfig

	env *JsEnv
}

// TODO rename file to script.go
// Script represents the script in a pipeline
func NewScript(path string, config *JSConfig) (*script, error) {
	name := filepath.Base(path)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("pipeline entry point does not exist: %s", path)
	}

	code := loadScript(path)
	env, err := NewJs(config, code)
	if err != nil {
		return nil, err
	}

	s := &script{
		name:   name,
		path:   path,
		env:    env,
		config: config,
	}

	if s.env.loop.vm.GlobalObject().Get("transform") == goja.Undefined() {
		return nil, fmt.Errorf("no transform function in script %s", s.name)
	}

	return s, nil
}

func loadScript(path string) string {
	// TODO configure esbuild
	scriptName := filepath.Base(path)
	dirName := filepath.Dir(path)
	log.Infof("Loading script %s from %s", scriptName, dirName)

	// TODO minify syntax dont work, because the entry functions will be renamed
	result := api.Build(api.BuildOptions{
		EntryPoints:      []string{scriptName},
		AbsWorkingDir:    dirName,
		Outfile:          "out.js",
		Format:           api.FormatESModule,
		Bundle:           true,
		Write:            false,
		TreeShaking:      api.TreeShakingFalse, // api.TreeShakingTrue
		MinifyWhitespace: true,
		// MinifyIdentifiers: true,
		MinifySyntax: true,
		Target:       api.ES2020, // api.ES5
		Sourcemap:    api.SourceMapNone,
	})
	// log.Infof("Build result: %+v", result)

	if len(result.Errors) > 0 {
		log.Errorf("[esbuid] cant load script: %s", strings.Join(api.FormatMessages(result.Errors, api.FormatMessagesOptions{Color: false}), ", "))
	}

	return string(result.OutputFiles[0].Contents)
}

type PipelineCtx struct {
	Name string
	Data map[string]string
}

func (s *script) getContext(vars map[string]string) *PipelineCtx {
	return &PipelineCtx{
		Name: s.name,
		Data: vars,
	}
}

func (s *script) Index(input string, vars map[string]string) map[string]string {
	ctx := s.getContext(vars)

	result, err := s.env.CallFunc("index", s.env.ToValue(ctx), s.env.ToValue(input))
	if err != nil {
		log.Errorf("Index return error %s", err)
	}

	if objRaw, ok := result.(*goja.Object); ok {
		if obj, ok := objRaw.Export().(map[string]string); ok {
			return obj
		}
	}

	return vars
}

func (s *script) Transform(input string, vars map[string]string) []map[string]string {
	ctx := s.getContext(vars)

	result, err := s.env.CallFunc("transform", s.env.ToValue(ctx), s.env.ToValue(input))
	if err != nil {
		log.Errorf("Transform return error %s", err)
	}

	var results []map[string]string

	if str, ok := result.Export().(string); ok {
		results = append(results, map[string]string{
			"filename": vars["target"],
			"result":   str,
		})
	} else if obj, ok := result.Export().(map[string]interface{}); ok {
		filename, _ := obj["filename"].(string)
		result, _ := obj["result"].(string)
		results = append(results, map[string]string{
			"filename": filename,
			"result":   result,
		})
	} else if arr, ok := result.Export().([]interface{}); ok {
		for _, item := range arr {
			if obj, ok := item.(map[string]interface{}); ok {
				filename, _ := obj["filename"].(string)
				result, _ := obj["result"].(string)
				results = append(results, map[string]string{
					"filename": filename,
					"result":   result,
				})
			}
		}
	}
	return results
}
