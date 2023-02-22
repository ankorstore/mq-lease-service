package middlewares

import (
	"strconv"
	"time"

	"github.com/ankorstore/gh-action-mq-lease-service/internal/metrics"
	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
)

func PrometheusMiddleware(app *fiber.App, metricsService metrics.Metrics, url string) fiber.Handler {
	requestsTotal := metricsService.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Count all http requests by status code, method and route name.",
	}, []string{"status_code", "method", "route_name"})

	requestDuration := metricsService.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Duration of all HTTP requests by status code, method and path.",
		Buckets: metrics.GetDefaultDurationBuckets(),
	}, []string{"status_code", "method", "route_name"})

	requestInFlight := metricsService.NewGaugeVec(prometheus.GaugeOpts{
		Name: "requests_in_progress_total",
		Help: "All the requests in progress",
	}, []string{"method"})

	app.Get(url, adaptor.HTTPHandler(metricsService.GetHTTPHandler())).Name("metrics")

	return func(c *fiber.Ctx) error {
		start := time.Now()
		method := c.Route().Method

		if c.Route().Path == url {
			// don't instrument the scrapping endpoint
			return c.Next()
		}

		requestInFlight.WithLabelValues(method).Inc()
		defer func() {
			requestInFlight.WithLabelValues(method).Dec()
		}()

		err := c.Next()
		status := fiber.StatusInternalServerError
		if err != nil {
			if e, ok := err.(*fiber.Error); ok {
				// Get correct error code from fiber.Error type
				status = e.Code
			}
		} else {
			status = c.Response().StatusCode()
		}

		routeName := c.Route().Name

		statusCode := strconv.Itoa(status)
		requestsTotal.WithLabelValues(statusCode, method, routeName).Inc()

		elapsed := float64(time.Since(start).Nanoseconds()) / 1e9
		requestDuration.WithLabelValues(statusCode, method, routeName).Observe(elapsed)

		return err
	}
}
