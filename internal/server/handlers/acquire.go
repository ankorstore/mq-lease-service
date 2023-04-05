package handlers

import (
	"github.com/ankorstore/gh-action-mq-lease-service/internal/lease"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

func Acquire(orchestrator lease.ProviderOrchestrator) func(c *fiber.Ctx) error {
	type acquireRequest struct {
		HeadSHA  string `json:"head_sha" validate:"required,min=1"`
		HeadRef  string `json:"head_ref" validate:"required,min=1,ghTempBranchRef"`
		Priority int    `json:"priority" validate:"required,number,min=1"`
	}

	validate := validator.New()
	registerGhTempBranchRefValidationRuleOrFail(validate)

	return func(c *fiber.Ctx) error {
		provider, fiberErr := getLeaseProviderOrFail(c, orchestrator)
		if provider == nil {
			return fiberErr
		}

		input := new(acquireRequest)
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
		}

		leaseRequestResponse, err := provider.Acquire(c.UserContext(), leaseRequest)
		if err != nil {
			return apiError(c, fiber.StatusConflict, "Couldn't acquire the lock", err.Error())
		}

		reqContext, err := provider.BuildRequestContext(c.UserContext(), leaseRequestResponse)
		if err != nil {
			return apiError(c, fiber.StatusInternalServerError, "Couldn't build request context", err.Error())
		}
		return c.Status(fiber.StatusOK).JSON(reqContext)
	}
}
