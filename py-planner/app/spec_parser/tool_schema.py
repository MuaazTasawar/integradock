"""
Pydantic shapes for parsed OpenAPI tools.

These mirror the Go models in go-engine/internal/models/tool.go exactly —
field names and casing must match what CreateAPIConnectionRequest /
ToolInput expect on the Go side (see models/tool.go), since the frontend
forwards this payload verbatim to POST /api/tools/connections.
"""
from typing import Any, Optional

from pydantic import BaseModel, Field


class ParsedTool(BaseModel):
    tool_name: str
    description: str
    http_method: str
    path_template: str
    parameters_schema: dict[str, Any] = Field(default_factory=dict)
    is_destructive: bool = False


class ParsedConnection(BaseModel):
    name: str
    base_url: str
    auth_type: str = "none"
    auth_config: dict[str, Any] = Field(default_factory=dict)
    spec_raw: dict[str, Any] = Field(default_factory=dict)
    tools: list[ParsedTool] = Field(default_factory=list)


class ParseSpecRequest(BaseModel):
    """Used for the JSON-body variant of /parse (spec pasted/fetched, not uploaded)."""
    connection_name: str
    base_url: Optional[str] = None
    spec: dict[str, Any]
    auth_type: str = "none"
    auth_config: dict[str, Any] = Field(default_factory=dict)