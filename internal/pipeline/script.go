package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"craeft/internal/log"
	"craeft/internal/pipeline/js"

	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
)

type script struct {
	name   string
	path   string
	config *js.JSConfig

	env *js.JsEnv
}

// Script represents the script in a pipeline
func NewScript(path string, config *js.JSConfig) (*script, error) {
	name := filepath.Base(path)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("pipeline entry point does not exist: %s", path)
	}

	code := loadScript(path)
	env, err := js.NewJs(config, code)
	if err != nil {
		return nil, err
	}

	s := &script{
		name:   name,
		path:   path,
		env:    env,
		config: config,
	}

	if s.env.VM().GlobalObject().Get("transform") == goja.Undefined() {
		return nil, fmt.Errorf("no transform function in script %s", s.name)
	}

	return s, nil
}

func loadScript(path string) string {
	// TODO configure esbuild
	scriptName := filepath.Base(path)
	dirName := filepath.Dir(path)

	// TODO minify syntax dont work, because the entry functions will be renamed
	// use transform instead of build?
	result := api.Build(api.BuildOptions{
		EntryPoints:      []string{scriptName},
		AbsWorkingDir:    dirName,
		Outfile:          "out.js",
		Format:           api.FormatCommonJS,
		Bundle:           true,
		Write:            false,
		TreeShaking:      api.TreeShakingFalse, // api.TreeShakingTrue
		MinifyWhitespace: false,
		// MinifyIdentifiers: true,
		MinifySyntax: false,
		Target:       api.ES2020, // better use api.ES2015?
		// Sourcemap: api.SourceMapNone,
		Sourcemap: api.SourceMapInline,
		Supported: map[string]bool{
			"for-await":       false,
			"async-generator": false,
		},
		// see js.go:setupRequire
		External: []string{"kv", "fs", "http", "md", "fasttpl", "tpl"},
	})

	if len(result.Errors) > 0 {
		log.Error(fmt.Errorf("esbuid can't load script: %s", strings.Join(api.FormatMessages(result.Errors, api.FormatMessagesOptions{Color: false}), ", ")))
		return ""
	}

	// TODO this could be written to debug files
	// log.Trace(fmt.Sprintf("esbuild generated: %s", result.OutputFiles[0].Contents))
	fmt.Println("saving debug file to %s", result.OutputFiles[0].Path)
	if err := os.WriteFile(result.OutputFiles[0].Path, []byte(result.OutputFiles[0].Contents), 0644); err != nil {
		panic(err)
	}
	return string(result.OutputFiles[0].Contents)
}

type PipelineCtx struct {
	Name string
	Data map[string]any
}

func (s *script) getContext(vars map[string]any) *PipelineCtx {
	return &PipelineCtx{
		Name: s.name,
		Data: vars,
	}
}

func (s *script) Index(input []byte, vars Vars) Vars {
	result, err := s.env.Call("index", vars.ToGojaObject(s.env.VM()), s.env.ToValue(input))
	if err != nil {
		log.Error(fmt.Errorf("%s:index return error: %s", s.config.Name, err), log.Tag{Key: log.SrcTag, Value: "pipeline"})
	}

	if objRaw, ok := result.(*goja.Object); ok {
		if obj, ok := objRaw.Export().(map[string]any); ok {
			return obj
		}
	}

	return map[string]any{}
}

func (s *script) Transform(input []byte, vars Vars) []map[string]string {
	// TODO pass env
	result, err := s.env.Call("transform", vars.ToGojaObject(s.env.VM()), s.env.ToValue(string(input)))

	var results []map[string]string

	if err != nil {
		log.Error(fmt.Errorf("%s.transform return error:\n%s", s.config.Name, err), log.Tag{Key: log.SrcTag, Value: "pipeline"})

		return []map[string]string{
			{"filename": "", "result": ""},
		}
	}

	if str, ok := result.Export().(string); ok {
		results = []map[string]string{
			{"filename": vars["target"].(string), "result": str},
		}
	} else if obj, ok := result.Export().(map[string]interface{}); ok {
		// TODO important! this could explode. ensure that the keys are present
		filename, _ := obj["filename"].(string)
		result, _ := obj["result"].(string)
		results = []map[string]string{
			{"filename": filename, "result": result},
		}
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
