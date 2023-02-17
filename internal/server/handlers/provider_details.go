package handlers

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/gofiber/fiber/v2"
)

func ProviderDetails(orchestrator lease.ProviderOrchestrator) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		provider, fiberErr := getLeaseProviderOrFail(c, orchestrator)
		if provider == nil {
			return fiberErr
		}
		return c.Status(fiber.StatusOK).JSON(provider)
	}
}
