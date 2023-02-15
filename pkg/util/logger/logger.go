package logger

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/rs/zerolog"
)

var (
	debug bool
	json  bool
)

type AppInfo interface {
	GetAppName() string
	GetCommit() string
	GetTag() string
}

func InitFlags() {
	flag.BoolVar(&debug, "log-debug", false, "Enable debug logging")
	flag.BoolVar(&json, "log-json", true, "Enable console logging format")
}

// locationHook adds the log location
type locationHook struct{}

func (h locationHook) Run(e *zerolog.Event, l zerolog.Level, msg string) {
	_, file, line, ok := runtime.Caller(3)
	if ok {
		e.Str("location", fmt.Sprintf("%s:%d", file, line))
	}
}

func New(info AppInfo) zerolog.Logger {
	var logOutput io.Writer
	var logLevel zerolog.Level

	logOutput = os.Stderr
	if !json {
		logOutput = zerolog.ConsoleWriter{Out: os.Stderr}
	}

	logLevel = zerolog.InfoLevel
	if debug {
		logLevel = zerolog.DebugLevel
	}

	// Default options that are overwritten by flags
	return zerolog.New(logOutput). // Stderr by default (k8s compat)
					Level(logLevel).      // info level by default
					Hook(locationHook{}). // Add caller information
					With().
					Timestamp().                     // Add timestamp to log
					Str("app", info.GetAppName()).   // Pass app name to context
					Str("build_tag", info.GetTag()). // Pass tag to context
					Logger()
}
