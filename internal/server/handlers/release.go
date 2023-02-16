package handlers

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

func Release(orchestrator lease.ProviderOrchestrator) func(c *fiber.Ctx) error {
	type releaseRequest struct {
		HeadSHA  string `json:"head_sha" validate:"required,min=1"`
		Priority int    `json:"priority" validate:"required,number,min=1"`
		Status   string `json:"status" validate:"required,oneof=success failure"`
	}

	validate := validator.New()

	return func(c *fiber.Ctx) error {
		provider, fiberErr := getLeaseProviderOrFail(c, orchestrator)
		if provider == nil {
			return fiberErr
		}

		input := new(releaseRequest)
		if ok, err := parseBodyOrFail(c, input); !ok {
			return err
		}
		if ok, err := validateInputOrFail(c, validate, input); !ok {
			return err
		}
		leaseRequest := &lease.Request{
			HeadSHA:  input.HeadSHA,
			Priority: input.Priority,
			Status:   &input.Status,
		}

		leaseRequestResponse, err := provider.Release(c.UserContext(), leaseRequest)
		if err != nil {
			return apiError(c, fiber.StatusBadRequest, "Couldn't release the lock", err.Error())
		}

		return c.Status(fiber.StatusOK).JSON(leaseRequestResponse)
	}
}
