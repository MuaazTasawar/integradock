package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MuaazTasawar/integradock/go-engine/internal/models"
)

// Engine ties together tool lookup, confirmation gating, and real HTTP execution.
type Engine struct {
	DB *pgxpool.Pool
}

func NewEngine(db *pgxpool.Pool) *Engine {
	return &Engine{DB: db}
}

// ErrConfirmationRequired signals the caller (execution_handler in Phase 6)
// that this step must not run until the user explicitly approves it.
var ErrConfirmationRequired = fmt.Errorf("executor: step requires confirmation before execution")

// RunTool loads a tool + its parent connection's auth config, enforces the
// destructive-action confirmation gate, and executes the real HTTP call.
// confirmed must be true for any tool where IsDestructive == true.
func (e *Engine) RunTool(ctx context.Context, toolID string, arguments map[string]any, confirmed bool) (*HTTPResult, error) {
	tool, conn, err := e.loadToolWithConnection(ctx, toolID)
	if err != nil {
		return nil, err
	}

	if tool.IsDestructive && !confirmed {
		return nil, ErrConfirmationRequired
	}

	var authCfg AuthConfig
	if err := json.Unmarshal(conn.AuthConfig, &authCfg); err != nil {
		return nil, fmt.Errorf("executor: failed to parse auth config for connection %s: %w", conn.ID, err)
	}

	spec := RequestSpec{
		BaseURL:      conn.BaseURL,
		HTTPMethod:   tool.HTTPMethod,
		PathTemplate: tool.PathTemplate,
		AuthType:     conn.AuthType,
		AuthConfig:   authCfg,
		Arguments:    arguments,
	}

	execCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	return Execute(execCtx, spec)
}

// IsDestructive is a lightweight lookup used by handlers/planner to decide
// whether to short-circuit into an "awaiting_confirmation" state before
// ever calling RunTool.
func (e *Engine) IsDestructive(ctx context.Context, toolID string) (bool, error) {
	var destructive bool
	row := e.DB.QueryRow(ctx, `SELECT is_destructive FROM tools WHERE id = $1`, toolID)
	if err := row.Scan(&destructive); err != nil {
		return false, fmt.Errorf("executor: failed to look up tool %s: %w", toolID, err)
	}
	return destructive, nil
}

func (e *Engine) loadToolWithConnection(ctx context.Context, toolID string) (*models.Tool, *models.APIConnection, error) {
	var t models.Tool
	trow := e.DB.QueryRow(ctx, `
		SELECT id, tenant_id, api_connection_id, tool_name, description, http_method, path_template, parameters_schema, is_destructive, created_at, updated_at
		FROM tools WHERE id = $1
	`, toolID)
	if err := trow.Scan(&t.ID, &t.TenantID, &t.APIConnectionID, &t.ToolName, &t.Description,
		&t.HTTPMethod, &t.PathTemplate, &t.ParametersSchema, &t.IsDestructive, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, nil, fmt.Errorf("executor: tool %s not found: %w", toolID, err)
	}

	var conn models.APIConnection
	crow := e.DB.QueryRow(ctx, `
		SELECT id, tenant_id, name, base_url, auth_type, auth_config, spec_raw, created_at, updated_at
		FROM api_connections WHERE id = $1
	`, t.APIConnectionID)
	if err := crow.Scan(&conn.ID, &conn.TenantID, &conn.Name, &conn.BaseURL, &conn.AuthType,
		&conn.AuthConfig, &conn.SpecRaw, &conn.CreatedAt, &conn.UpdatedAt); err != nil {
		return nil, nil, fmt.Errorf("executor: api connection %s not found: %w", t.APIConnectionID, err)
	}

	return &t, &conn, nil
}
