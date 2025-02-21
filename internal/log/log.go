package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

var (
	logger zerolog.Logger
	once   sync.Once
)

type TagKey int

const (
	SrcTag TagKey = iota
	PipelineTag
)

var tagLabels = map[TagKey]string{
	SrcTag:      "src",
	PipelineTag: "pipeline",
}

func (tk TagKey) String() string {
	return tagLabels[tk]
}

type Tag struct {
	Key   TagKey
	Value string
}

func applyTags(event *zerolog.Event, tags ...Tag) *zerolog.Event {
	for _, tag := range tags {
		event = event.Str(tag.Key.String(), tag.Value)
	}
	return event
}

func Trace(msg string, tags ...Tag) {
	event := logger.Trace()
	applyTags(event, tags...).Msg(msg)
}

func Debug(msg string, tags ...Tag) {
	event := logger.Debug()
	applyTags(event, tags...).Msg(msg)
}

func Info(msg string, tags ...Tag) {
	event := logger.Info()
	applyTags(event, tags...).Msg(msg)
}

func Warn(msg string, tags ...Tag) {
	event := logger.Warn()
	applyTags(event, tags...).Msg(msg)
}
func Error(err error, tags ...Tag) {
	event := logger.Error()
	applyTags(event, tags...).Msg(err.Error())
}
func Fatal(msg string, tags ...Tag) {
	event := logger.Fatal()
	applyTags(event, tags...).Msg(msg)
}

func Panic(msg string, tags ...Tag) {
	event := logger.Panic()
	applyTags(event, tags...).Msg(msg)
}

func init() {
	once.Do(func() {

		lw := &LevelWriter{os.Stdout, os.Stderr}
		output := zerolog.ConsoleWriter{Out: lw}
		logger = zerolog.New(output).With().Timestamp().Logger()
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	})
}

type LevelWriter struct {
	io.Writer
	ErrorWriter io.Writer
}

func (lw *LevelWriter) WriteLevel(l zerolog.Level, p []byte) (n int, err error) {
	w := lw.Writer
	if l == zerolog.ErrorLevel {
		w = lw.ErrorWriter
	}
	return w.Write(p)
}

func Print(msg string, args ...interface{}) {
	msg = strings.TrimSpace(msg)
	msg = " ➤ " + msg + "\n"
	fmt.Printf(msg, args...)
}
