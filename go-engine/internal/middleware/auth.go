package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// InternalAuth checks a shared-secret header for service-to-service calls
// (py-planner -> go-engine, frontend -> go-engine in the MVP demo).
// Replace with per-tenant API keys / JWT before any real multi-tenant rollout.
func InternalAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if secret == "" {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "server misconfigured: INTERNAL_API_SECRET not set",
			})
		}

		provided := c.Get("X-Internal-Secret")
		if provided == "" || provided != secret {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing or invalid X-Internal-Secret header",
			})
		}

		return c.Next()
	}
}
