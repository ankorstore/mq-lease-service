package logger

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

const (
	TraceparentHeaderName = "w3c-traceparent"
)

func FiberMiddleware(logger zerolog.Logger) fiber.Handler {

	return func(c *fiber.Ctx) error {

		log := logger.With().Logger()

		// if traceparent is present, add it to the log
		if traceparent := c.Get(TraceparentHeaderName, ""); traceparent != "" {
			log = logger.With().Str("req_trace_parent", traceparent).Logger()
		}

		ctx := log.WithContext(c.UserContext())
		c.SetUserContext(ctx)

		start := time.Now()

		msg := ""
		err := c.Next()
		if err != nil {
			msg = err.Error()
			_ = c.SendStatus(fiber.StatusInternalServerError)
		}

		log = log.With().
			Int("status", c.Response().StatusCode()).
			Str("latency", time.Since(start).String()).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Str("protocol", c.Protocol()).
			Logger()

		// Set loglevel based on status code
		if c.Response().StatusCode() >= fiber.StatusInternalServerError {
			log.Error().Msg(msg)
		} else if c.Response().StatusCode() >= fiber.StatusBadRequest {
			log.Warn().Msg(msg)
		} else {
			log.Info().Msg(msg)
		}

		return err
	}
}
