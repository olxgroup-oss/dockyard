package utils

import (
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

type LoggingConfig struct {
	Level *string `mapstructure:"LEVEL"`
}

type SimpleTextFormatter struct {
}

// Formats in the form 2022-08-01T15:03:55+0530 INFO Logging something
func (f *SimpleTextFormatter) Format(entry *log.Entry) ([]byte, error) {
	renderedString := fmt.Sprintf(
		"%v %v %v\n",
		entry.Time.Format("2006-01-02T15:04:05-0700"),
		strings.ToUpper(entry.Level.String()),
		entry.Message,
	)

	return []byte(renderedString), nil
}

func SetupLogger(level log.Level) {

	file, _ := os.OpenFile("dockyard.log", os.O_CREATE|os.O_WRONLY, 0666)

	log.SetFormatter(new(SimpleTextFormatter))
	log.SetLevel(level)
	log.SetOutput(file)
}
