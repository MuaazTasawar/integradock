-- 003_executions.sql
-- Execution runs (a full agent session) and individual tool call steps

CREATE TABLE IF NOT EXISTS execution_runs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_prompt TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending | running | awaiting_confirmation | completed | failed | cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS execution_steps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    execution_run_id UUID NOT NULL REFERENCES execution_runs(id) ON DELETE CASCADE,
    step_index INT NOT NULL,
    tool_id UUID REFERENCES tools(id) ON DELETE SET NULL,
    tool_name VARCHAR(255) NOT NULL,
    arguments JSONB NOT NULL DEFAULT '{}'::jsonb,
    requires_confirmation BOOLEAN NOT NULL DEFAULT false,
    confirmed BOOLEAN NOT NULL DEFAULT false,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending | awaiting_confirmation | executing | success | error | skipped
    result JSONB,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (execution_run_id, step_index)
);

CREATE INDEX IF NOT EXISTS idx_execution_steps_run ON execution_steps (execution_run_id);
CREATE INDEX IF NOT EXISTS idx_execution_runs_tenant ON execution_runs (tenant_id);