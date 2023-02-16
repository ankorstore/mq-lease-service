package logger

import (
	"fmt"
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

		msg := "Request"
		err := c.Next()
		if err != nil {
			msg = err.Error()
			_ = c.SendStatus(fiber.StatusInternalServerError)
		}

		log = log.With().
			Int("req_status", c.Response().StatusCode()).
			Str("req_latency", fmt.Sprintf("%.3f", float64(time.Since(start).Microseconds())/1000)).
			Str("req_method", c.Method()).
			Str("req_ip", c.IP()).
			Str("req_path", c.Path()).
			Str("req_proto", c.Protocol()).
			Str("req_user_agent", c.Get(fiber.HeaderUserAgent)).
			Logger()

		// Set loglevel based on status code
		switch {
		case c.Response().StatusCode() >= fiber.StatusInternalServerError:
			log.Error().Msg(msg)
		case c.Response().StatusCode() >= fiber.StatusBadRequest:
			log.Warn().Msg(msg)
		default:
			log.Info().Msg(msg)
		}

		return err
	}
}
