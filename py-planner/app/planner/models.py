"""
Internal (provider-agnostic-ish) shapes used by the agent loop and streamed
to the frontend as Server-Sent Events. Every event has a "type" discriminator
the frontend's TracePanel switches on.
"""
from typing import Any, Literal, Optional

from pydantic import BaseModel, Field


class ToolDef(BaseModel):
    """A tool as registered in go-engine, used both for LLM function-calling
    definitions and for step bookkeeping."""
    id: str
    tool_name: str
    description: str
    http_method: str
    path_template: str
    parameters_schema: dict[str, Any] = Field(default_factory=dict)
    is_destructive: bool = False


class PlanStartRequest(BaseModel):
    tenant_id: str
    user_prompt: str
    llm_provider: Optional[str] = None  # overrides DEFAULT_LLM_PROVIDER if set


class PlanConfirmRequest(BaseModel):
    tenant_id: str
    run_id: str
    step_id: str
    approved: bool
    llm_provider: Optional[str] = None


EventType = Literal[
    "run_started",
    "assistant_message",
    "tool_call_proposed",
    "awaiting_confirmation",
    "tool_executing",
    "tool_result",
    "final_answer",
    "run_completed",
    "error",
]


class TraceEvent(BaseModel):
    """One SSE frame. `type` drives how the frontend renders it in the trace panel."""
    type: EventType
    run_id: str
    step_id: Optional[str] = None
    tool_name: Optional[str] = None
    arguments: Optional[dict[str, Any]] = None
    result: Optional[Any] = None
    message: Optional[str] = None
    is_destructive: Optional[bool] = None

    def to_sse(self) -> str:
        return f"data: {self.model_dump_json(exclude_none=True)}\n\n"