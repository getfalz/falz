package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"golang.org/x/term"
)

var (
	logger zerolog.Logger
	once   sync.Once
)

func init() {
	once.Do(func() {

		var writer io.Writer
		writer = &LevelWriter{os.Stdout, os.Stderr}
		if term.IsTerminal(int(os.Stdout.Fd())) {
			writer = zerolog.ConsoleWriter{Out: writer}
		}

		logger = zerolog.New(writer).With().Timestamp().Logger()
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	})
}

func Logger() *zerolog.Logger { return &logger }

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

func Print(msg string, args ...any) {
	msg = strings.TrimSpace(msg)
	msg = " ➤ " + msg + "\n"
	fmt.Printf(msg, args...)
}

type TagKey int

const (
	SrcTag TagKey = iota
	PipelineTag

	None
	Pipeline
	Build
	Index
)

var tagLabels = map[TagKey]string{
	SrcTag:      "src",
	PipelineTag: "pipeline",
}

func (tk TagKey) String() string {
	return tagLabels[tk]
}

func DDebug(tag TagKey) *zerolog.Event {
	l := logger.With().Str("component", tag.String()).Logger()
	return l.Debug()
}

func TTrace(tag TagKey) *zerolog.Event {
	l := logger.With().Str("component", tag.String()).Logger()
	return l.Trace()
}
func IInfo(tag TagKey) *zerolog.Event {
	l := logger.With().Str("component", tag.String()).Logger()
	return l.Info()
}

func EError(tag TagKey) *zerolog.Event {
	l := logger.With().Str("component", tag.String()).Logger()
	return l.Error()
}

type Tag struct {
	Key   TagKey
	Value string
}

// TODO deprecate
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
