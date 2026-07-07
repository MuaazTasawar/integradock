"""
Turns a raw OpenAPI 3.x spec (dict, already loaded from JSON or YAML) into a
flat list of callable tools with JSON-Schema parameter definitions suitable
for LLM function-calling.
"""
import re
from typing import Any

import yaml

from app.spec_parser.tool_schema import ParsedConnection, ParsedTool

WRITE_METHODS = {"POST", "PUT", "PATCH", "DELETE"}
SUPPORTED_METHODS = {"GET", "POST", "PUT", "PATCH", "DELETE"}


def load_spec_bytes(raw: bytes) -> dict[str, Any]:
    """Loads spec content as JSON first, falling back to YAML (OpenAPI specs are valid YAML supersets of JSON)."""
    text = raw.decode("utf-8")
    try:
        import json
        return json.loads(text)
    except ValueError:
        return yaml.safe_load(text)


def _infer_base_url(spec: dict[str, Any], fallback: str | None) -> str:
    servers = spec.get("servers") or []
    if servers and isinstance(servers, list) and "url" in servers[0]:
        return servers[0]["url"]
    if fallback:
        return fallback
    raise ValueError("openapi_parser: no servers[] in spec and no base_url override provided")


def _tool_name_from_operation(method: str, path: str, operation: dict[str, Any]) -> str:
    op_id = operation.get("operationId")
    if op_id:
        # normalize to snake_case, LLM-function-name-safe
        name = re.sub(r"[^a-zA-Z0-9_]", "_", op_id)
        name = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", name).lower()
        return name

    # fallback: derive from method + path, e.g. GET /orders/{id} -> get_orders_by_id
    cleaned = re.sub(r"\{([^}]+)\}", r"by_\1", path)
    cleaned = re.sub(r"[^a-zA-Z0-9]+", "_", cleaned).strip("_").lower()
    return f"{method.lower()}_{cleaned}"


def _is_destructive(method: str, operation: dict[str, Any]) -> bool:
    if "x-destructive" in operation:
        return bool(operation["x-destructive"])
    return method.upper() in WRITE_METHODS


def _resolve_ref(spec: dict[str, Any], ref: str) -> dict[str, Any]:
    """Resolves a local '#/components/schemas/Foo'-style $ref."""
    if not ref.startswith("#/"):
        return {}
    parts = ref.lstrip("#/").split("/")
    node: Any = spec
    for p in parts:
        node = node.get(p, {}) if isinstance(node, dict) else {}
    return node if isinstance(node, dict) else {}


def _resolve_schema(spec: dict[str, Any], schema: dict[str, Any]) -> dict[str, Any]:
    if "$ref" in schema:
        return _resolve_schema(spec, _resolve_ref(spec, schema["$ref"]))
    return schema


def _build_parameters_schema(spec: dict[str, Any], operation: dict[str, Any]) -> dict[str, Any]:
    """
    Builds a single JSON Schema object combining path params, query params,
    and request body fields into one flat "properties" map — this is the
    shape the LLM function-calling tool definition expects, and matches
    what executor.RequestSpec.Arguments consumes (path params get substituted
    into path_template, everything else becomes query params or "body").
    """
    properties: dict[str, Any] = {}
    required: list[str] = []

    for param in operation.get("parameters", []):
        if "$ref" in param:
            param = _resolve_ref(spec, param["$ref"])
        name = param.get("name")
        if not name:
            continue
        param_schema = _resolve_schema(spec, param.get("schema", {"type": "string"}))
        properties[name] = {
            "type": param_schema.get("type", "string"),
            "description": param.get("description", f"{param.get('in', 'param')} parameter '{name}'"),
        }
        if param.get("example") is not None:
            properties[name]["example"] = param["example"]
        if param.get("required"):
            required.append(name)

    request_body = operation.get("requestBody")
    if request_body:
        content = request_body.get("content", {})
        json_content = content.get("application/json", {})
        body_schema = _resolve_schema(spec, json_content.get("schema", {}))
        if body_schema:
            properties["body"] = {
                "type": "object",
                "description": "Request body payload",
                "properties": body_schema.get("properties", {}),
            }
            body_required = body_schema.get("required", [])
            if request_body.get("required") and body_required:
                required.append("body")

    return {
        "type": "object",
        "properties": properties,
        "required": required,
    }


def parse_openapi_spec(
    spec: dict[str, Any],
    connection_name: str,
    base_url_override: str | None = None,
) -> ParsedConnection:
    """Main entrypoint: raw spec dict -> ParsedConnection with a flat tool list."""
    base_url = _infer_base_url(spec, base_url_override)
    paths = spec.get("paths", {})

    tools: list[ParsedTool] = []
    for path, path_item in paths.items():
        if not isinstance(path_item, dict):
            continue
        for method, operation in path_item.items():
            method_upper = method.upper()
            if method_upper not in SUPPORTED_METHODS or not isinstance(operation, dict):
                continue

            tool_name = _tool_name_from_operation(method_upper, path, operation)
            description = (
                operation.get("summary")
                or operation.get("description")
                or f"{method_upper} {path}"
            )
            parameters_schema = _build_parameters_schema(spec, operation)
            destructive = _is_destructive(method_upper, operation)

            tools.append(
                ParsedTool(
                    tool_name=tool_name,
                    description=description.strip(),
                    http_method=method_upper,
                    path_template=path,
                    parameters_schema=parameters_schema,
                    is_destructive=destructive,
                )
            )

    if not tools:
        raise ValueError("openapi_parser: no callable operations found in spec.paths")

    return ParsedConnection(
        name=connection_name,
        base_url=base_url,
        spec_raw=spec,
        tools=tools,
    )