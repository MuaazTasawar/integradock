package models

import (
	"encoding/json"
	"time"
)

// ExecutionRun represents one end-to-end agent session triggered by a user prompt.
type ExecutionRun struct {
	ID         string    `json:"id" db:"id"`
	TenantID   string    `json:"tenant_id" db:"tenant_id"`
	UserPrompt string    `json:"user_prompt" db:"user_prompt"`
	Status     string    `json:"status" db:"status"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// ExecutionStep represents a single planned tool call within an ExecutionRun.
type ExecutionStep struct {
	ID                   string          `json:"id" db:"id"`
	ExecutionRunID       string          `json:"execution_run_id" db:"execution_run_id"`
	StepIndex            int             `json:"step_index" db:"step_index"`
	ToolID               *string         `json:"tool_id" db:"tool_id"`
	ToolName             string          `json:"tool_name" db:"tool_name"`
	Arguments            json.RawMessage `json:"arguments" db:"arguments"`
	RequiresConfirmation bool            `json:"requires_confirmation" db:"requires_confirmation"`
	Confirmed            bool            `json:"confirmed" db:"confirmed"`
	Status               string          `json:"status" db:"status"`
	Result               json.RawMessage `json:"result" db:"result"`
	ErrorMessage         *string         `json:"error_message" db:"error_message"`
	StartedAt            *time.Time      `json:"started_at" db:"started_at"`
	CompletedAt          *time.Time      `json:"completed_at" db:"completed_at"`
	CreatedAt            time.Time       `json:"created_at" db:"created_at"`
}

// CreateExecutionRunRequest is the payload from py-planner (or frontend) to start a new run.
type CreateExecutionRunRequest struct {
	TenantID   string `json:"tenant_id" validate:"required"`
	UserPrompt string `json:"user_prompt" validate:"required"`
}

// ConfirmStepRequest is used to approve or reject a step awaiting confirmation.
type ConfirmStepRequest struct {
	Approved bool `json:"approved"`
}
