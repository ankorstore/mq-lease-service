package logger

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/rs/zerolog"
)

type AppInfo interface {
	GetAppName() string
	GetCommit() string
	GetTag() string
}

// locationHook adds the log location
type locationHook struct{}

func (h locationHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	_, file, line, ok := runtime.Caller(3)
	if ok {
		e.Str("location", fmt.Sprintf("%s:%d", file, line))
	}
}

type NewOpts struct {
	AppInfo AppInfo
	Debug   bool
	JSON    bool
}

func New(opts NewOpts) zerolog.Logger {
	var logOutput io.Writer
	var logLevel zerolog.Level

	logOutput = os.Stderr
	if !opts.JSON {
		logOutput = zerolog.ConsoleWriter{Out: os.Stderr}
	}

	logLevel = zerolog.InfoLevel
	if opts.Debug {
		logLevel = zerolog.DebugLevel
	}

	// Default options that are overwritten by flags
	return zerolog.New(logOutput). // Stderr by default (k8s compat)
					Level(logLevel).      // info level by default
					Hook(locationHook{}). // Add caller information
					With().
					Timestamp().                             // Add timestamp to log
					Str("app", opts.AppInfo.GetAppName()).   // Pass app name to context
					Str("build_tag", opts.AppInfo.GetTag()). // Pass tag to context
					Logger()
}
