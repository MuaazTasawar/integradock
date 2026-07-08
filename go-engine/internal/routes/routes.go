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

// Register mounts all HTTP + websocket routes on the given Fiber app.
func Register(app *fiber.App, deps Deps) {
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "integradock-go-engine",
		})
	})

	tenantHandler := handlers.NewTenantHandler(deps.DB)
	toolHandler := handlers.NewToolHandler(deps.DB)
	engine := executor.NewEngine(deps.DB)
	executionHandler := handlers.NewExecutionHandler(deps.DB, deps.Redis, engine)
	wsHandler := handlers.NewWebsocketHandler(deps.Redis)

	api := app.Group("/api", appmiddleware.InternalAuth(deps.InternalAPISecret))

	tenants := api.Group("/tenants")
	tenants.Post("/", tenantHandler.Create)
	tenants.Get("/", tenantHandler.List)
	tenants.Get("/:slug", tenantHandler.GetBySlug)

	tools := api.Group("/tools")
	tools.Post("/connections", toolHandler.CreateConnection)
	tools.Get("/", toolHandler.ListByTenant)
	tools.Get("/:id", toolHandler.GetByID)

	executions := api.Group("/executions")
	executions.Post("/", executionHandler.CreateRun)
	executions.Get("/:id", executionHandler.GetRun)
	executions.Patch("/:id", executionHandler.UpdateRunStatus)
	executions.Post("/:id/steps", executionHandler.CreateStep)
	executions.Post("/:id/steps/:step_id/execute", executionHandler.ExecuteStep)
	executions.Post("/:id/steps/:step_id/confirm", executionHandler.ConfirmStep)

	// Websocket streaming is intentionally outside the /api InternalAuth group -
	// the frontend connects directly from the browser and cannot attach a
	// custom X-Internal-Secret header during the WS handshake. run_id acts as
	// the bearer token for the demo; add a signed short-lived ws ticket before
	// any real multi-tenant deployment.
	ws := app.Group("/ws")
	ws.Use(handlers.UpgradeMiddleware)
	ws.Get("/executions/:run_id", wsHandler.StreamExecution())
}
