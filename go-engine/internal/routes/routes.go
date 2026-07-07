package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/MuaazTasawar/integradock/go-engine/internal/executor"
	"github.com/MuaazTasawar/integradock/go-engine/internal/handlers"
	appmiddleware "github.com/MuaazTasawar/integradock/go-engine/internal/middleware"
)

// Deps bundles the shared dependencies routes need to wire up handlers.
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

	tenantHandler := handlers.NewTenantHandler(deps.DB)
	toolHandler := handlers.NewToolHandler(deps.DB)
	_ = executor.NewEngine(deps.DB) // wired into execution_handler in Phase 6

	api := app.Group("/api", appmiddleware.InternalAuth(deps.InternalAPISecret))

	tenants := api.Group("/tenants")
	tenants.Post("/", tenantHandler.Create)
	tenants.Get("/", tenantHandler.List)
	tenants.Get("/:slug", tenantHandler.GetBySlug)

	tools := api.Group("/tools")
	tools.Post("/connections", toolHandler.CreateConnection)
	tools.Get("/", toolHandler.ListByTenant)
	tools.Get("/:id", toolHandler.GetByID)

	// Execution routes are attached in Phase 6 once redisstate + websocket
	// streaming exist (execution_handler.go, websocket_handler.go).
	_ = api.Group("/executions")
}
