package logger

import (
	"os"
	"time"

	"github.com/charmbracelet/log"
)

func InitLogger() *log.Logger {
	logger := log.NewWithOptions(os.Stderr, log.Options{
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      time.Kitchen,
	})

	return logger
}
