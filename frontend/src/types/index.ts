export type TraceEventType =
  | "run_started"
  | "assistant_message"
  | "tool_call_proposed"
  | "awaiting_confirmation"
  | "tool_executing"
  | "tool_result"
  | "final_answer"
  | "run_completed"
  | "error";

// Mirrors py-planner/app/planner/models.py::TraceEvent (streamed over SSE
// from /plan/start and /plan/confirm).
export interface TraceEvent {
  type: TraceEventType;
  run_id: string;
  step_id?: string;
  tool_name?: string;
  arguments?: Record<string, unknown>;
  result?: unknown;
  message?: string;
  is_destructive?: boolean;
}

// Mirrors go-engine/internal/redisstate/state.go::StepEvent (streamed over
// the /ws/executions/:run_id websocket).
export interface StepEvent {
  type:
    | "step_created"
    | "step_executing"
    | "step_result"
    | "step_error"
    | "run_status_changed";
  run_id: string;
  step_id?: string;
  tool_name?: string;
  status?: string;
  result?: unknown;
  error_message?: string;
  timestamp: string;
}

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
}

export interface ParsedTool {
  tool_name: string;
  description: string;
  http_method: string;
  path_template: string;
  parameters_schema: Record<string, unknown>;
  is_destructive: boolean;
}

export interface ParsedConnection {
  name: string;
  base_url: string;
  auth_type: string;
  auth_config: Record<string, unknown>;
  spec_raw: Record<string, unknown>;
  tools: ParsedTool[];
}

export interface RegisteredTool {
  id: string;
  tenant_id: string;
  api_connection_id: string;
  tool_name: string;
  description: string;
  http_method: string;
  path_template: string;
  parameters_schema: Record<string, unknown>;
  is_destructive: boolean;
}

// A step currently paused waiting on a human decision, surfaced by the
// ConfirmationModal. Carries what the ChatPanel needs to resume the run.
export interface PendingConfirmation {
  runId: string;
  stepId: string;
  toolName: string;
  arguments: Record<string, unknown>;
}