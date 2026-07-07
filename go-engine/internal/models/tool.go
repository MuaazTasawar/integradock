package models

import (
	"encoding/json"
	"time"
)

// APIConnection represents one uploaded/registered external API (an OpenAPI spec source).
type APIConnection struct {
	ID         string          `json:"id" db:"id"`
	TenantID   string          `json:"tenant_id" db:"tenant_id"`
	Name       string          `json:"name" db:"name"`
	BaseURL    string          `json:"base_url" db:"base_url"`
	AuthType   string          `json:"auth_type" db:"auth_type"`
	AuthConfig json.RawMessage `json:"auth_config" db:"auth_config"`
	SpecRaw    json.RawMessage `json:"spec_raw" db:"spec_raw"`
	CreatedAt  time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at" db:"updated_at"`
}

// Tool represents a single callable operation derived from an OpenAPI spec,
// exposed to the LLM as a function-calling tool definition.
type Tool struct {
	ID               string          `json:"id" db:"id"`
	TenantID         string          `json:"tenant_id" db:"tenant_id"`
	APIConnectionID  string          `json:"api_connection_id" db:"api_connection_id"`
	ToolName         string          `json:"tool_name" db:"tool_name"`
	Description      string          `json:"description" db:"description"`
	HTTPMethod       string          `json:"http_method" db:"http_method"`
	PathTemplate     string          `json:"path_template" db:"path_template"`
	ParametersSchema json.RawMessage `json:"parameters_schema" db:"parameters_schema"`
	IsDestructive    bool            `json:"is_destructive" db:"is_destructive"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}

// CreateAPIConnectionRequest is the payload for registering a new API connection
// (used after the Python service has parsed an OpenAPI spec into tool definitions).
type CreateAPIConnectionRequest struct {
	TenantID   string          `json:"tenant_id" validate:"required"`
	Name       string          `json:"name" validate:"required"`
	BaseURL    string          `json:"base_url" validate:"required"`
	AuthType   string          `json:"auth_type"`
	AuthConfig json.RawMessage `json:"auth_config"`
	SpecRaw    json.RawMessage `json:"spec_raw"`
	Tools      []ToolInput     `json:"tools" validate:"required,dive"`
}

// ToolInput is a single parsed tool definition coming from py-planner's spec parser.
type ToolInput struct {
	ToolName         string          `json:"tool_name" validate:"required"`
	Description      string          `json:"description"`
	HTTPMethod       string          `json:"http_method" validate:"required"`
	PathTemplate     string          `json:"path_template" validate:"required"`
	ParametersSchema json.RawMessage `json:"parameters_schema"`
	IsDestructive    bool            `json:"is_destructive"`
}
