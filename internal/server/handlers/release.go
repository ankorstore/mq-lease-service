package handlers

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

func Release(orchestrator lease.ProviderOrchestrator) func(c *fiber.Ctx) error {
	type releaseRequest struct {
		HeadSHA  string `json:"head_sha" validate:"required,min=1"`
		HeadRef  string `json:"head_ref" validate:"required,min=1,ghTempBranchRef"`
		Priority int    `json:"priority" validate:"required,number,min=1"`
		Status   string `json:"status" validate:"required,oneof=success failure"`
	}

	validate := validator.New()
	registerGhTempBranchRefValidationRuleOrFail(validate)

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
			HeadRef:  input.HeadRef,
			Priority: input.Priority,
			Status:   &input.Status,
		}

		leaseRequestResponse, err := provider.Release(c.UserContext(), leaseRequest)
		if err != nil {
			log.Ctx(c.UserContext()).Error().Err(err).Msg("Couldn't release the lock")
			return apiError(c, fiber.StatusBadRequest, "Couldn't release the lock", err.Error())
		}

		reqContext, err := provider.BuildlRequestContext(c.UserContext(), leaseRequestResponse)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "Couldn't build request context", err.Error())
		}
		return c.Status(fiber.StatusOK).JSON(reqContext)
	}
}
