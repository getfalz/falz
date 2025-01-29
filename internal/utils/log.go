package utils

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
)

type CraeftFormatter struct {
	log.TextFormatter
}

func (f *CraeftFormatter) Format(entry *log.Entry) ([]byte, error) {
	var levelSymbol string
	switch entry.Level {
	case log.InfoLevel:
		levelSymbol = "➤"
	case log.DebugLevel:
		levelSymbol = "\t›"
	case log.TraceLevel:
		levelSymbol = "\t\t*"

	case log.WarnLevel:
		levelSymbol = "!"
	case log.ErrorLevel:
		levelSymbol = "X"
	case log.FatalLevel:
		levelSymbol = "#"
	default:
		levelSymbol = "➤"
	}

	entry.Message = strings.TrimSpace(entry.Message)
	entry.Message = levelSymbol + " " + entry.Message

	f.DisableTimestamp = true

	// TODO Format is striping spaces
	return f.TextFormatter.Format(entry)
}

func Print(msg string, args ...interface{}) {
	msg = strings.TrimSpace(msg)
	msg = " ➤ " + msg + "\n"
	fmt.Printf(msg, args...)
}
