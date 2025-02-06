package handlers

import (
	"github.com/ankorstore/mq-lease-service/internal/lease"
	"github.com/ankorstore/mq-lease-service/internal/storage"
	"github.com/gofiber/fiber/v2"
)

func Liveness() func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	}
}

func Readiness(storage storage.Storage[*lease.ProviderState]) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		if passed := storage.HealthCheck(c.UserContext(), func() *lease.ProviderState {
			return lease.NewProviderState(lease.NewProviderStateOpts{
				ID: "test-healthcheck",
			})
		}); !passed {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		return c.SendStatus(fiber.StatusOK)
	}
}
