package handlers

import (
	"github.com/ankorstore/mq-lease-service/internal/lease"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type apiErrorResponse struct {
	Error        string `json:"error"`
	ErrorContext any    `json:"error_context,omitempty"`
}

func getLeaseProviderOrFail(c *fiber.Ctx, orchestrator lease.ProviderOrchestrator) (lease.Provider, error) {
	owner := c.Params("owner")
	repo := c.Params("repo")
	baseRef := c.Params("baseRef")

	log.Ctx(c.UserContext()).UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.
			Str("repo_owner", owner).
			Str("repo_name", repo).
			Str("repo_baseRef", baseRef)
	})

	provider, err := orchestrator.Get(owner, repo, baseRef)
	if err != nil {
		log.Ctx(c.UserContext()).Error().Err(err).Msg("Error when retrieving provider")
		return nil, apiError(c, fiber.StatusNotFound, err.Error(), nil)
	}

	return provider, nil
}

func parseBodyOrFail(c *fiber.Ctx, out interface{}) (bool, error) {
	if err := c.BodyParser(out); err != nil {
		log.Ctx(c.UserContext()).Error().Err(err).Msg("Error when parsing request body")
		return false, apiError(c, fiber.StatusUnprocessableEntity, err.Error(), nil)
	}
	return true, nil
}

type inputValidationError struct {
	FailedField string `json:"failed_field"`
	Tag         string `json:"tag"`
	Value       string `json:"value"`
}

func ghTempBranchRefNameValidation(fl validator.FieldLevel) bool {
	return lease.ValidateGHTempRef(fl.Field().String())
}

func registerGhTempBranchRefValidationRuleOrFail(validate *validator.Validate) {
	if err := validate.RegisterValidation("ghTempBranchRef", ghTempBranchRefNameValidation); err != nil {
		panic("Error when trying to register GH branch ref validation rule in validator: " + err.Error())
	}
}

func validateInputOrFail(c *fiber.Ctx, validate *validator.Validate, subject any) (bool, error) {
	errs := validateInput(validate, subject)
	if len(errs) > 0 {
		return false, apiError(c, fiber.StatusBadRequest, "Invalid request", errs)
	}
	return true, nil
}

func validateInput(validate *validator.Validate, subject any) []*inputValidationError {
	var errs []*inputValidationError
	err := validate.Struct(subject)
	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			errs = append(errs, &inputValidationError{
				FailedField: err.StructNamespace(),
				Tag:         err.Tag(),
				Value:       err.Param(),
			})
		}
	}
	return errs
}

func apiError(c *fiber.Ctx, status int, err string, errCtx any) error {
	return c.Status(status).JSON(apiErrorResponse{Error: err, ErrorContext: errCtx})
}
