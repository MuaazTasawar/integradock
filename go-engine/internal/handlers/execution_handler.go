package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/MuaazTasawar/integradock/go-engine/internal/executor"
	"github.com/MuaazTasawar/integradock/go-engine/internal/models"
	"github.com/MuaazTasawar/integradock/go-engine/internal/redisstate"
)

// ExecutionHandler owns run + step lifecycle endpoints, and coordinates
// between Postgres (source of truth), the executor.Engine (real HTTP calls),
// and Redis pub/sub (live trace panel streaming).
type ExecutionHandler struct {
	DB      *pgxpool.Pool
	Redis   *redis.Client
	Engine  *executor.Engine
}

func NewExecutionHandler(db *pgxpool.Pool, rdb *redis.Client, engine *executor.Engine) *ExecutionHandler {
	return &ExecutionHandler{DB: db, Redis: rdb, Engine: engine}
}

// CreateRun starts a new execution run. POST /api/executions
func (h *ExecutionHandler) CreateRun(c *fiber.Ctx) error {
	var req models.CreateExecutionRunRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.TenantID == "" || req.UserPrompt == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tenant_id and user_prompt are required"})
	}

	ctx := context.Background()
	var run models.ExecutionRun
	row := h.DB.QueryRow(ctx, `
		INSERT INTO execution_runs (tenant_id, user_prompt, status)
		VALUES ($1, $2, 'pending')
		RETURNING id, tenant_id, user_prompt, status, created_at, updated_at
	`, req.TenantID, req.UserPrompt)

	if err := row.Scan(&run.ID, &run.TenantID, &run.UserPrompt, &run.Status, &run.CreatedAt, &run.UpdatedAt); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create execution run: " + err.Error()})
	}

	_ = redisstate.SetRunState(ctx, h.Redis, run.ID, run.Status)

	return c.Status(fiber.StatusCreated).JSON(run)
}

// GetRun returns a run with all its steps, in the shape agent_loop.py expects:
// {"run": {...}, "steps": [...]}. GET /api/executions/:id
func (h *ExecutionHandler) GetRun(c *fiber.Ctx) error {
	runID := c.Params("id")
	ctx := context.Background()

	var run models.ExecutionRun
	row := h.DB.QueryRow(ctx, `
		SELECT id, tenant_id, user_prompt, status, created_at, updated_at
		FROM execution_runs WHERE id = $1
	`, runID)
	if err := row.Scan(&run.ID, &run.TenantID, &run.UserPrompt, &run.Status, &run.CreatedAt, &run.UpdatedAt); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "execution run not found"})
	}

	rows, err := h.DB.Query(ctx, `
		SELECT id, execution_run_id, step_index, tool_id, tool_name, arguments, requires_confirmation, confirmed, status, result, error_message, started_at, completed_at, created_at
		FROM execution_steps WHERE execution_run_id = $1 ORDER BY step_index ASC
	`, runID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load steps: " + err.Error()})
	}
	defer rows.Close()

	steps := make([]models.ExecutionStep, 0)
	for rows.Next() {
		var s models.ExecutionStep
		if err := rows.Scan(&s.ID, &s.ExecutionRunID, &s.StepIndex, &s.ToolID, &s.ToolName, &s.Arguments,
			&s.RequiresConfirmation, &s.Confirmed, &s.Status, &s.Result, &s.ErrorMessage, &s.StartedAt, &s.CompletedAt, &s.CreatedAt); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to scan step: " + err.Error()})
		}
		steps = append(steps, s)
	}

	return c.JSON(fiber.Map{"run": run, "steps": steps})
}

// UpdateRunStatus patches the run's status. PATCH /api/executions/:id
func (h *ExecutionHandler) UpdateRunStatus(c *fiber.Ctx) error {
	runID := c.Params("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := c.BodyParser(&body); err != nil || body.Status == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "status is required"})
	}

	ctx := context.Background()
	var run models.ExecutionRun
	row := h.DB.QueryRow(ctx, `
		UPDATE execution_runs SET status = $1, updated_at = now() WHERE id = $2
		RETURNING id, tenant_id, user_prompt, status, created_at, updated_at
	`, body.Status, runID)
	if err := row.Scan(&run.ID, &run.TenantID, &run.UserPrompt, &run.Status, &run.CreatedAt, &run.UpdatedAt); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "execution run not found"})
	}

	_ = redisstate.SetRunState(ctx, h.Redis, run.ID, run.Status)
	_ = redisstate.Publish(ctx, h.Redis, run.ID, redisstate.StepEvent{
		Type:   "run_status_changed",
		Status: run.Status,
	})

	return c.JSON(run)
}

// CreateStep records a planned tool call for a run. POST /api/executions/:id/steps
func (h *ExecutionHandler) CreateStep(c *fiber.Ctx) error {
	runID := c.Params("id")
	var body struct {
		ToolID               *string         `json:"tool_id"`
		ToolName             string          `json:"tool_name"`
		Arguments            json.RawMessage `json:"arguments"`
		RequiresConfirmation bool            `json:"requires_confirmation"`
	}
	if err := c.BodyParser(&body); err != nil || body.ToolName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tool_name is required"})
	}
	if body.Arguments == nil {
		body.Arguments = json.RawMessage(`{}`)
	}

	ctx := context.Background()

	var nextIndex int
	if err := h.DB.QueryRow(ctx, `
		SELECT COALESCE(MAX(step_index), -1) + 1 FROM execution_steps WHERE execution_run_id = $1
	`, runID).Scan(&nextIndex); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to compute step index: " + err.Error()})
	}

	initialStatus := "pending"
	if body.RequiresConfirmation {
		initialStatus = "awaiting_confirmation"
	}

	var s models.ExecutionStep
	row := h.DB.QueryRow(ctx, `
		INSERT INTO execution_steps (execution_run_id, step_index, tool_id, tool_name, arguments, requires_confirmation, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, execution_run_id, step_index, tool_id, tool_name, arguments, requires_confirmation, confirmed, status, result, error_message, started_at, completed_at, created_at
	`, runID, nextIndex, body.ToolID, body.ToolName, body.Arguments, body.RequiresConfirmation, initialStatus)

	if err := row.Scan(&s.ID, &s.ExecutionRunID, &s.StepIndex, &s.ToolID, &s.ToolName, &s.Arguments,
		&s.RequiresConfirmation, &s.Confirmed, &s.Status, &s.Result, &s.ErrorMessage, &s.StartedAt, &s.CompletedAt, &s.CreatedAt); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create step: " + err.Error()})
	}

	_ = redisstate.Publish(ctx, h.Redis, runID, redisstate.StepEvent{
		Type: "step_created", StepID: s.ID, ToolName: s.ToolName, Status: s.Status,
	})

	return c.Status(fiber.StatusCreated).JSON(s)
}

// ExecuteStep runs a non-destructive step immediately. POST /api/executions/:id/steps/:step_id/execute
func (h *ExecutionHandler) ExecuteStep(c *fiber.Ctx) error {
	runID := c.Params("id")
	stepID := c.Params("step_id")
	ctx := context.Background()

	step, err := h.loadStep(ctx, stepID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "execution step not found"})
	}
	if step.ToolID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "step has no associated tool_id and cannot be executed"})
	}

	var args map[string]any
	if err := json.Unmarshal(step.Arguments, &args); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to parse step arguments: " + err.Error()})
	}

	return h.runAndPersist(c, ctx, runID, step, args, false)
}

// ConfirmStep approves or rejects a destructive step. POST /api/executions/:id/steps/:step_id/confirm
func (h *ExecutionHandler) ConfirmStep(c *fiber.Ctx) error {
	runID := c.Params("id")
	stepID := c.Params("step_id")
	ctx := context.Background()

	var body models.ConfirmStepRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	step, err := h.loadStep(ctx, stepID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "execution step not found"})
	}

	if !body.Approved {
		if _, err := h.DB.Exec(ctx, `
			UPDATE execution_steps SET status = 'skipped', confirmed = false WHERE id = $1
		`, stepID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to mark step skipped: " + err.Error()})
		}
		_ = redisstate.Publish(ctx, h.Redis, runID, redisstate.StepEvent{
			Type: "step_result", StepID: stepID, ToolName: step.ToolName, Status: "skipped",
		})
		return c.JSON(fiber.Map{"id": stepID, "tool_name": step.ToolName, "status": "skipped"})
	}

	if _, err := h.DB.Exec(ctx, `UPDATE execution_steps SET confirmed = true WHERE id = $1`, stepID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to mark step confirmed: " + err.Error()})
	}
	step.Confirmed = true

	if step.ToolID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "step has no associated tool_id and cannot be executed"})
	}

	var args map[string]any
	if err := json.Unmarshal(step.Arguments, &args); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to parse step arguments: " + err.Error()})
	}

	return h.runAndPersist(c, ctx, runID, step, args, true)
}

// runAndPersist calls the executor engine, persists the result, and publishes
// live trace events. Shared by ExecuteStep and ConfirmStep(approved=true).
func (h *ExecutionHandler) runAndPersist(c *fiber.Ctx, ctx context.Context, runID string, step *models.ExecutionStep, args map[string]any, confirmed bool) error {
	now := time.Now().UTC()
	if _, err := h.DB.Exec(ctx, `
		UPDATE execution_steps SET status = 'executing', started_at = $2 WHERE id = $1
	`, step.ID, now); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to mark step executing: " + err.Error()})
	}
	_ = redisstate.Publish(ctx, h.Redis, runID, redisstate.StepEvent{
		Type: "step_executing", StepID: step.ID, ToolName: step.ToolName, Status: "executing",
	})

	result, err := h.Engine.RunTool(ctx, *step.ToolID, args, confirmed)
	completedAt := time.Now().UTC()

	if err != nil {
		if errors.Is(err, executor.ErrConfirmationRequired) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "step requires confirmation before it can be executed"})
		}
		if _, dbErr := h.DB.Exec(ctx, `
			UPDATE execution_steps SET status = 'error', error_message = $2, completed_at = $3 WHERE id = $1
		`, step.ID, err.Error(), completedAt); dbErr != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to persist step error: " + dbErr.Error()})
		}
		_ = redisstate.Publish(ctx, h.Redis, runID, redisstate.StepEvent{
			Type: "step_error", StepID: step.ID, ToolName: step.ToolName, Status: "error", ErrorMessage: err.Error(),
		})
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"id": step.ID, "tool_name": step.ToolName, "status": "error", "error_message": err.Error(),
		})
	}

	if _, dbErr := h.DB.Exec(ctx, `
		UPDATE execution_steps SET status = 'success', result = $2, completed_at = $3 WHERE id = $1
	`, step.ID, result.Body, completedAt); dbErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to persist step result: " + dbErr.Error()})
	}

	_ = redisstate.Publish(ctx, h.Redis, runID, redisstate.StepEvent{
		Type: "step_result", StepID: step.ID, ToolName: step.ToolName, Status: "success", Result: result.Body,
	})

	return c.JSON(fiber.Map{
		"id": step.ID, "tool_name": step.ToolName, "status": "success", "result": result.Body,
	})
}

func (h *ExecutionHandler) loadStep(ctx context.Context, stepID string) (*models.ExecutionStep, error) {
	var s models.ExecutionStep
	row := h.DB.QueryRow(ctx, `
		SELECT id, execution_run_id, step_index, tool_id, tool_name, arguments, requires_confirmation, confirmed, status, result, error_message, started_at, completed_at, created_at
		FROM execution_steps WHERE id = $1
	`, stepID)
	err := row.Scan(&s.ID, &s.ExecutionRunID, &s.StepIndex, &s.ToolID, &s.ToolName, &s.Arguments,
		&s.RequiresConfirmation, &s.Confirmed, &s.Status, &s.Result, &s.ErrorMessage, &s.StartedAt, &s.CompletedAt, &s.CreatedAt)
	return &s, err
}