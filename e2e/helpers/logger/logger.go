package logger

import (
	"io"

	"github.com/rs/zerolog"
)

// NewDummyLogger returns a logger which do nothing when used
func NewDummyLogger() *zerolog.Logger {
	logger := zerolog.New(io.Discard).Level(zerolog.NoLevel)
	return &logger
}
