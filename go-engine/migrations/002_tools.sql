-- 002_tools.sql
-- Tool registry: one row per callable operation parsed from an OpenAPI spec

CREATE TABLE IF NOT EXISTS api_connections (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    base_url VARCHAR(1024) NOT NULL,
    auth_type VARCHAR(50) NOT NULL DEFAULT 'none', -- none | bearer | api_key | basic
    auth_config JSONB NOT NULL DEFAULT '{}'::jsonb, -- header names, env var refs (never raw secrets)
    spec_raw JSONB NOT NULL DEFAULT '{}'::jsonb,    -- parsed OpenAPI spec (sanitized)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tools (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    api_connection_id UUID NOT NULL REFERENCES api_connections(id) ON DELETE CASCADE,
    tool_name VARCHAR(255) NOT NULL,          -- normalized, LLM-facing name e.g. "create_customer"
    description TEXT NOT NULL,
    http_method VARCHAR(10) NOT NULL,         -- GET, POST, PUT, PATCH, DELETE
    path_template VARCHAR(1024) NOT NULL,     -- e.g. /v1/customers/{id}
    parameters_schema JSONB NOT NULL DEFAULT '{}'::jsonb, -- JSON Schema for function-calling
    is_destructive BOOLEAN NOT NULL DEFAULT false,        -- true for write/delete -> confirmation gate
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, tool_name)
);

CREATE INDEX IF NOT EXISTS idx_tools_tenant ON tools (tenant_id);
CREATE INDEX IF NOT EXISTS idx_tools_api_connection ON tools (api_connection_id);
CREATE INDEX IF NOT EXISTS idx_api_connections_tenant ON api_connections (tenant_id);