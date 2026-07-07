package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	appmiddleware "github.com/MuaazTasawar/integradock/go-engine/internal/middleware"
)

// Deps bundles the shared dependencies routes need to wire up handlers.
// Handler structs are added to this in later phases (tool registry in Phase 2,
// execution + websocket streaming in Phase 6) without changing this file's shape.
type Deps struct {
	DB                *pgxpool.Pool
	Redis             *redis.Client
	InternalAPISecret string
}

// Register mounts all HTTP routes on the given Fiber app.
func Register(app *fiber.App, deps Deps) {
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "integradock-go-engine",
		})
	})

	api := app.Group("/api", appmiddleware.InternalAuth(deps.InternalAPISecret))

	// Route groups are declared here so the tree is stable across phases;
	// handlers are attached to these groups starting in Phase 2.
	_ = api.Group("/tenants")
	_ = api.Group("/tools")
	_ = api.Group("/executions")
}
