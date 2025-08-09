package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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
func NewScript(name, path string, config *js.JSConfig, code string) (*script, error) {
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

func (s *script) Index(parCtx context.Context, input []byte, vars Vars) error {
	// TODO set timeout in config
	ctx, cancel := context.WithTimeout(parCtx, 1*time.Second)
	defer cancel()

	_, err := s.env.Call(ctx, "index", vars.ToGojaObject(s.env.VM()), s.env.ToValue(input))
	if err != nil {
		log.Error(fmt.Errorf("%s:index error: %s", s.config.Name, err), log.Tag{Key: log.SrcTag, Value: "pipeline"})
	}

	// if objRaw, ok := result.(*goja.Object); ok {
	// 	if obj, ok := objRaw.Export().(map[string]any); ok {
	// 		return obj, nil
	// 	}
	// }

	return nil
}

func (s *script) Transform(parCtx context.Context, input []byte, vars Vars) ([]map[string]string, error) {
	// TODO set timeout in config
	ctx, cancel := context.WithTimeout(parCtx, 1*time.Second)
	defer cancel()

	result, err := s.env.Call(ctx, "transform", vars.ToGojaObject(s.env.VM()), s.env.ToValue(string(input)))

	if err != nil {
		log.Error(fmt.Errorf("%s.transform error:\n%s", s.config.Name, err), log.Tag{Key: log.SrcTag, Value: "pipeline"})

		return []map[string]string{
			{"filename": "", "result": ""},
		}, fmt.Errorf("%s:transform error: %w", s.config.Name, err)
	}

	// TODO important! this could explode. ensure that the keys are present
	switch v := result.Export().(type) {
	case string:
		return []map[string]string{
			{"filename": vars["target"].(string), "result": v},
		}, nil
	case map[string]interface{}:
		filename, _ := v["filename"].(string)
		result, _ := v["result"].(string)
		return []map[string]string{
			{"filename": filename, "result": result},
		}, nil
	case []interface{}:
		out := make([]map[string]string, 0, len(v))
		for _, item := range v {
			if obj, ok := item.(map[string]interface{}); ok {
				filename, _ := obj["filename"].(string)
				result, _ := obj["result"].(string)
				out = append(out, map[string]string{
					"filename": filename,
					"result":   result,
				})
			}
		}
		return out, nil
	default:
		return []map[string]string{{"filename": "", "result": ""}}, nil
	}
}

func loadScript(entryDir, entryFile string) (string, error) {
	// TODO minify syntax dont work, because the entry functions will be renamed
	// use transform instead of build?
	result := api.Build(api.BuildOptions{
		EntryPoints:      []string{entryFile},
		AbsWorkingDir:    entryDir,
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
		return "", fmt.Errorf("esbuid can't load script: %s", strings.Join(api.FormatMessages(result.Errors, api.FormatMessagesOptions{Color: false}), ", "))
	}

	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("esbuild produced no output")
	}

	// TODO this could be written to debug files
	// log.Trace(fmt.Sprintf("esbuild generated: %s", result.OutputFiles[0].Contents))
	fmt.Println("saving debug file to %s", result.OutputFiles[0].Path)
	_ = os.WriteFile(result.OutputFiles[0].Path, []byte(result.OutputFiles[0].Contents), 0o644)

	return string(result.OutputFiles[0].Contents), nil
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
