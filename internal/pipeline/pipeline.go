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
		// Platform:    api.PlatformNode,
		Format: api.FormatESModule,
		// Format: api.FormatCommonJS,
		Bundle:      true,
		Write:       false,
		TreeShaking: api.TreeShakingFalse,
	})
	log.Infof("Build result: %+v", result)

	if len(result.Errors) > 0 {
		log.Errorf("[esbuid] cant load script: %s", strings.Join(api.FormatMessages(result.Errors, api.FormatMessagesOptions{Color: false}), ", "))
	}

	return string(result.OutputFiles[0].Contents)
}

func (s *script) getContext(vars map[string]any) goja.Value {
	ctx := s.vm.NewObject()
	for k, v := range vars {
		ctx.Set(k, s.vm.ToValue(v))
	}
	return ctx
}

func (s *script) Index(input string) {

}

func (s *script) Transform(input string, vars map[string]any) string {

	transform, ok := goja.AssertFunction(s.vm.Get("transform"))
	if !ok {
		log.Errorf("No transform function in script %s", s.name)
	}

	ctx := s.getContext(vars)

	res, err := transform(ctx, s.vm.ToValue(input))
	if err != nil {
		log.Errorf("Transform return error %s", err)
	}

	return res.String()
}
