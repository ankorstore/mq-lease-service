package handlers

import (
	"fmt"
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

func Acquire(orchestrator lease.LeaseProviderOrchestrator) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		leaseRequest := &lease.LeaseRequest{}

		if err := c.BodyParser(leaseRequest); err != nil {
			errMsg := "Error when parsing acquire request body"
			log.Error().Err(err).Msg(errMsg)
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"error": errMsg,
			})
		}

		fmt.Println(leaseRequest)

		leaseRequestResponse, err := orchestrator.Get(c.Params("owner"), c.Params("repo"), c.Params("baseRef")).Acquire(leaseRequest)
		if err != nil {
			return c.SendStatus(fiber.StatusConflict)
		}

		return c.Status(fiber.StatusOK).JSON(leaseRequestResponse)
	}
}
