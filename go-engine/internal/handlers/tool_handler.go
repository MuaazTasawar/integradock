package handlers

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MuaazTasawar/integradock/go-engine/internal/models"
)

// ToolHandler owns endpoints for registering API connections + their parsed
// tools, and for listing/fetching tools used by the planning loop.
type ToolHandler struct {
	DB *pgxpool.Pool
}

func NewToolHandler(db *pgxpool.Pool) *ToolHandler {
	return &ToolHandler{DB: db}
}

// CreateConnection registers a new API connection + bulk-inserts its parsed tools.
// Called by py-planner right after it parses an uploaded OpenAPI spec.
// POST /api/tools/connections
func (h *ToolHandler) CreateConnection(c *fiber.Ctx) error {
	var req models.CreateAPIConnectionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.TenantID == "" || req.Name == "" || req.BaseURL == "" || len(req.Tools) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "tenant_id, name, base_url, and at least one tool are required",
		})
	}
	if req.AuthType == "" {
		req.AuthType = "none"
	}
	if req.AuthConfig == nil {
		req.AuthConfig = json.RawMessage(`{}`)
	}
	if req.SpecRaw == nil {
		req.SpecRaw = json.RawMessage(`{}`)
	}

	ctx := context.Background()
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to start transaction: " + err.Error()})
	}
	defer tx.Rollback(ctx)

	var conn models.APIConnection
	row := tx.QueryRow(ctx, `
		INSERT INTO api_connections (tenant_id, name, base_url, auth_type, auth_config, spec_raw)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, name, base_url, auth_type, auth_config, spec_raw, created_at, updated_at
	`, req.TenantID, req.Name, req.BaseURL, req.AuthType, req.AuthConfig, req.SpecRaw)

	if err := row.Scan(&conn.ID, &conn.TenantID, &conn.Name, &conn.BaseURL, &conn.AuthType,
		&conn.AuthConfig, &conn.SpecRaw, &conn.CreatedAt, &conn.UpdatedAt); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create api connection: " + err.Error()})
	}

	insertedTools := make([]models.Tool, 0, len(req.Tools))
	for _, ti := range req.Tools {
		if ti.ParametersSchema == nil {
			ti.ParametersSchema = json.RawMessage(`{}`)
		}
		var t models.Tool
		trow := tx.QueryRow(ctx, `
			INSERT INTO tools (tenant_id, api_connection_id, tool_name, description, http_method, path_template, parameters_schema, is_destructive)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, tenant_id, api_connection_id, tool_name, description, http_method, path_template, parameters_schema, is_destructive, created_at, updated_at
		`, req.TenantID, conn.ID, ti.ToolName, ti.Description, ti.HTTPMethod, ti.PathTemplate, ti.ParametersSchema, ti.IsDestructive)

		if err := trow.Scan(&t.ID, &t.TenantID, &t.APIConnectionID, &t.ToolName, &t.Description,
			&t.HTTPMethod, &t.PathTemplate, &t.ParametersSchema, &t.IsDestructive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to insert tool '" + ti.ToolName + "': " + err.Error()})
		}
		insertedTools = append(insertedTools, t)
	}

	if err := tx.Commit(ctx); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to commit transaction: " + err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"connection": conn,
		"tools":      insertedTools,
	})
}

// ListByTenant returns all tools registered for a tenant, in the shape the
// LLM planning loop needs for function-calling definitions.
// GET /api/tools?tenant_id=...
func (h *ToolHandler) ListByTenant(c *fiber.Ctx) error {
	tenantID := c.Query("tenant_id")
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant_id query param is required"})
	}

	ctx := context.Background()
	rows, err := h.DB.Query(ctx, `
		SELECT id, tenant_id, api_connection_id, tool_name, description, http_method, path_template, parameters_schema, is_destructive, created_at, updated_at
		FROM tools
		WHERE tenant_id = $1
		ORDER BY tool_name ASC
	`, tenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list tools: " + err.Error()})
	}
	defer rows.Close()

	tools := make([]models.Tool, 0)
	for rows.Next() {
		var t models.Tool
		if err := rows.Scan(&t.ID, &t.TenantID, &t.APIConnectionID, &t.ToolName, &t.Description,
			&t.HTTPMethod, &t.PathTemplate, &t.ParametersSchema, &t.IsDestructive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to scan tool: " + err.Error()})
		}
		tools = append(tools, t)
	}

	return c.JSON(tools)
}

// GetByID fetches a single tool with its API connection auth config attached —
// used internally by the executor right before making the real HTTP call.
// GET /api/tools/:id
func (h *ToolHandler) GetByID(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := context.Background()

	var t models.Tool
	row := h.DB.QueryRow(ctx, `
		SELECT id, tenant_id, api_connection_id, tool_name, description, http_method, path_template, parameters_schema, is_destructive, created_at, updated_at
		FROM tools WHERE id = $1
	`, id)

	if err := row.Scan(&t.ID, &t.TenantID, &t.APIConnectionID, &t.ToolName, &t.Description,
		&t.HTTPMethod, &t.PathTemplate, &t.ParametersSchema, &t.IsDestructive, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tool not found"})
	}

	return c.JSON(t)
}
