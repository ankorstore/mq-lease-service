package handlers

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/gofiber/fiber/v2"
)

func ProviderList(orchestrator lease.ProviderOrchestrator) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(orchestrator.GetAll())
	}
}
