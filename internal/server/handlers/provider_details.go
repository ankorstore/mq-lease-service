package handlers

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/gofiber/fiber/v2"
)

func ProviderDetails(orchestrator lease.LeaseProviderOrchestrator) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		provider := orchestrator.Get(c.Params("owner"), c.Params("repo"), c.Params("baseRef"))
		return c.Status(fiber.StatusOK).JSON(provider.GetKnown())
	}
}
