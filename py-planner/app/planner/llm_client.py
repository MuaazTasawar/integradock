"""
Raw function-calling clients for Anthropic and Gemini — no LangChain, no
agent framework. agent_loop.py talks to this module through one normalized
interface: call_llm(...) -> LLMResponse.

Internal conversation format (the `messages` argument) is Anthropic-native:
    [{"role": "user" | "assistant", "content": [ {..block..}, ... ]}, ...]
Blocks are one of:
    {"type": "text", "text": "..."}
    {"type": "tool_use", "id": "...", "name": "...", "input": {...}}
    {"type": "tool_result", "tool_use_id": "...", "content": "..."}
For the Gemini path, these are converted on the fly.
"""
import json
import logging
from typing import Any

from anthropic import AsyncAnthropic
from pydantic import BaseModel

from app.config import get_settings

logger = logging.getLogger("integradock.llm_client")

ANTHROPIC_MODEL = "claude-sonnet-5"
GEMINI_MODEL = "gemini-flash-latest"

MAX_TOKENS = 1024

def _sanitize_schema_for_gemini(schema: dict[str, Any]) -> dict[str, Any]:
    """
    Gemini's function-calling schema is stricter than plain JSON Schema and
    rejects fields like 'example'. Recursively strips unsupported keys so the
    same parameters_schema (built for Anthropic-style tool defs) also works
    for Gemini.
    """
    if not isinstance(schema, dict):
        return schema

    ALLOWED_KEYS = {"type", "description", "properties", "required", "items", "enum"}
    cleaned: dict[str, Any] = {}

    for key, value in schema.items():
        if key not in ALLOWED_KEYS:
            continue
        if key == "properties" and isinstance(value, dict):
            cleaned[key] = {k: _sanitize_schema_for_gemini(v) for k, v in value.items()}
        elif key == "items" and isinstance(value, dict):
            cleaned[key] = _sanitize_schema_for_gemini(value)
        else:
            cleaned[key] = value

    return cleaned

class ToolCall(BaseModel):
    id: str
    name: str
    arguments: dict[str, Any]


class LLMResponse(BaseModel):
    text: str = ""
    tool_calls: list[ToolCall] = []
    stop_reason: str = ""


def tools_to_anthropic_format(tools: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [
        {
            "name": t["tool_name"],
            "description": t["description"] or t["tool_name"],
            "input_schema": t["parameters_schema"] or {"type": "object", "properties": {}},
        }
        for t in tools
    ]


async def call_anthropic(
    system_prompt: str,
    messages: list[dict[str, Any]],
    tools: list[dict[str, Any]],
) -> LLMResponse:
    settings = get_settings()
    if not settings.anthropic_api_key:
        raise RuntimeError("llm_client: ANTHROPIC_API_KEY is not set")

    client = AsyncAnthropic(api_key=settings.anthropic_api_key)

    resp = await client.messages.create(
        model=ANTHROPIC_MODEL,
        max_tokens=MAX_TOKENS,
        system=system_prompt,
        messages=messages,
        tools=tools_to_anthropic_format(tools) if tools else [],
    )

    text_parts: list[str] = []
    tool_calls: list[ToolCall] = []
    for block in resp.content:
        if block.type == "text":
            text_parts.append(block.text)
        elif block.type == "tool_use":
            tool_calls.append(ToolCall(id=block.id, name=block.name, arguments=block.input or {}))

    return LLMResponse(text="\n".join(text_parts).strip(), tool_calls=tool_calls, stop_reason=resp.stop_reason or "")


def _anthropic_messages_to_gemini_contents(
    system_prompt: str, messages: list[dict[str, Any]]
) -> list[dict[str, Any]]:
    """Best-effort conversion of the Anthropic-native message list into
    Gemini `contents` — enough fidelity for the demo's tool-call/tool-result loop."""
    contents: list[dict[str, Any]] = [
        {"role": "user", "parts": [{"text": system_prompt}]},
        {"role": "model", "parts": [{"text": "Understood."}]},
    ]

    for msg in messages:
        role = "model" if msg["role"] == "assistant" else "user"
        parts: list[dict[str, Any]] = []
        for block in msg["content"]:
            if block["type"] == "text":
                parts.append({"text": block["text"]})
            elif block["type"] == "tool_use":
                parts.append({"function_call": {"name": block["name"], "args": block["input"]}})
            elif block["type"] == "tool_result":
                content = block["content"]
                parts.append({
                    "function_response": {
                        "name": block.get("_tool_name", "tool_result"),
                        "response": {"result": content},
                    }
                })
        if parts:
            contents.append({"role": role, "parts": parts})

    return contents


async def call_gemini(
    system_prompt: str,
    messages: list[dict[str, Any]],
    tools: list[dict[str, Any]],
) -> LLMResponse:
    settings = get_settings()
    if not settings.gemini_api_key:
        raise RuntimeError("llm_client: GEMINI_API_KEY is not set")

    import google.generativeai as genai

    genai.configure(api_key=settings.gemini_api_key)

    function_declarations = [
        {
            "name": t["tool_name"],
            "description": t["description"] or t["tool_name"],
            "parameters": _sanitize_schema_for_gemini(t["parameters_schema"] or {"type": "object", "properties": {}}),
        }
        for t in tools
    ]

    model = genai.GenerativeModel(
        model_name=GEMINI_MODEL,
        tools=[{"function_declarations": function_declarations}] if function_declarations else None,
    )

    contents = _anthropic_messages_to_gemini_contents(system_prompt, messages)
    resp = await model.generate_content_async(contents)

    text_parts: list[str] = []
    tool_calls: list[ToolCall] = []
    candidate = resp.candidates[0] if resp.candidates else None
    if candidate:
        for part in candidate.content.parts:
            if getattr(part, "text", None):
                text_parts.append(part.text)
            fc = getattr(part, "function_call", None)
            if fc and fc.name:
                tool_calls.append(
                    ToolCall(id=f"gemini_call_{len(tool_calls)}", name=fc.name, arguments=dict(fc.args or {}))
                )

    stop_reason = "tool_use" if tool_calls else "end_turn"
    return LLMResponse(text="\n".join(text_parts).strip(), tool_calls=tool_calls, stop_reason=stop_reason)


async def call_llm(
    provider: str,
    system_prompt: str,
    messages: list[dict[str, Any]],
    tools: list[dict[str, Any]],
) -> LLMResponse:
    if provider == "gemini":
        return await call_gemini(system_prompt, messages, tools)
    if provider == "anthropic":
        return await call_anthropic(system_prompt, messages, tools)
    raise ValueError(f"llm_client: unknown provider '{provider}' (expected 'anthropic' or 'gemini')")