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

type PipelineScript interface {
	Index(string)
	Transform(input string, vars map[string]any) string
}

type script struct {
	name   string
	path   string
	script string
	vm     *goja.Runtime
	prg    *goja.Program
}

func NewScript(path string) (*script, error) {
	name := filepath.Base(path)

	vm := goja.New()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("pipeline entry point does not exist: %s", path)
	}

	return &script{
		name: name,
		path: path,
		vm:   vm,
	}, nil
}

func (s *script) Load() error {
	// TODO check if already loaded
	code := s.loadScript()
	log.Infof("Loaded script %s", code)

	// Compile the JavaScript code
	p, err := goja.Compile(s.name, code, false)
	if err != nil {
		return err
	}

	// TODO maybe, we dont need to keep the program around
	s.prg = p

	_, err = s.vm.RunProgram(p)
	if err != nil {
		return err
	}

	// Ensure the 'transform' function is available
	if s.vm.GlobalObject().Get("transform") == goja.Undefined() {
		return fmt.Errorf("no transform function in script %s", s.name)
	}

	return nil
}

func (s *script) loadScript() string {
	// TODO configure esbuild
	scriptName := filepath.Base(s.path)
	dirName := filepath.Dir(s.path)
	log.Infof("Loading script %s from %s", scriptName, dirName)

	result := api.Build(api.BuildOptions{
		EntryPoints:   []string{scriptName},
		AbsWorkingDir: dirName,
		Outfile:       "out.js",
		Format:        api.FormatESModule,
		Bundle:        true,
		Write:         false,
		TreeShaking:   api.TreeShakingFalse,
	})
	log.Infof("Build result: %+v", result)

	if len(result.Errors) > 0 {
		log.Errorf("[esbuid] cant load script: %s", strings.Join(api.FormatMessages(result.Errors, api.FormatMessagesOptions{Color: false}), ", "))
	}

	return string(result.OutputFiles[0].Contents)
}

func (s *script) getContext(vars map[string]string) goja.Value {
	ctx := s.vm.NewObject()
	ctx.Set("name", s.name)
	ctx.Set("data", vars)

	return ctx
}

func (s *script) Index(input string, vars map[string]string) map[string]string {
	index, ok := goja.AssertFunction(s.vm.Get("index"))
	if !ok {
		log.Infof("No index function in script %s", s.name)
		return vars
	}

	ctx := s.getContext(vars)

	result, err := index(ctx, s.vm.ToValue(input))
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
	transform, ok := goja.AssertFunction(s.vm.Get("transform"))
	if !ok {
		log.Errorf("No transform function in script %s", s.name)
	}

	ctx := s.getContext(vars)

	result, err := transform(ctx, s.vm.ToValue(input))
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
