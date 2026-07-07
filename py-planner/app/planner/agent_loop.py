"""
The core plan -> act -> observe loop. No LangChain — this talks to llm_client
directly and drives go_engine_client to actually execute tools.

Two entrypoints:
  start()  - brand new run from a user prompt
  resume() - continue a run after a human approved/rejected a destructive step

Both are async generators yielding TraceEvent, consumed by routers/plan.py
and streamed to the frontend over SSE.
"""
import logging
from typing import Any, AsyncGenerator, Optional

from app.clients.go_engine_client import GoEngineClient, GoEngineError
from app.config import get_settings
from app.planner.llm_client import ToolCall, call_llm
from app.planner.models import TraceEvent, ToolDef
from app.planner.prompts import SYSTEM_PROMPT, build_tenant_context

logger = logging.getLogger("integradock.agent_loop")

MAX_ITERATIONS = 8


class AgentLoop:
    def __init__(self) -> None:
        self.go_engine = GoEngineClient()
        self.settings = get_settings()

    async def _load_tools(self, tenant_id: str) -> list[ToolDef]:
        raw_tools = await self.go_engine.list_tools(tenant_id)
        return [ToolDef(**t) for t in raw_tools]

    def _tool_defs_for_llm(self, tools: list[ToolDef]) -> list[dict[str, Any]]:
        return [t.model_dump() for t in tools]

    def _find_tool(self, tools: list[ToolDef], name: str) -> Optional[ToolDef]:
        for t in tools:
            if t.tool_name == name:
                return t
        return None

    async def _rebuild_messages_from_history(self, run_id: str, user_prompt: str) -> list[dict[str, Any]]:
        """Reconstructs the Anthropic-native message list from steps already
        stored in go-engine. Uses each step's own id as a synthetic, consistent
        tool_use_id for both halves of that step's turn (see llm_client.py docstring)."""
        run = await self.go_engine.get_execution_run(run_id)
        steps = run.get("steps", []) or []

        messages: list[dict[str, Any]] = [{"role": "user", "content": [{"type": "text", "text": user_prompt}]}]

        for step in steps:
            if step.get("status") == "pending":
                # never executed (e.g. a rejected sibling) - skip from history
                continue
            messages.append({
                "role": "assistant",
                "content": [{
                    "type": "tool_use",
                    "id": step["id"],
                    "name": step["tool_name"],
                    "input": step.get("arguments") or {},
                }],
            })
            result_content = step.get("result") if step.get("status") == "success" else {
                "error": step.get("error_message") or "step was not approved and was skipped"
            }
            messages.append({
                "role": "user",
                "content": [{
                    "type": "tool_result",
                    "tool_use_id": step["id"],
                    "content": result_content,
                    "_tool_name": step["tool_name"],
                }],
            })

        return messages

    async def start(self, tenant_id: str, user_prompt: str, provider: Optional[str] = None) -> AsyncGenerator[TraceEvent, None]:
        provider = provider or self.settings.default_llm_provider

        try:
            tenant = await self.go_engine.get_tenant(tenant_id) if False else None  # slug lookup not needed here
        except GoEngineError:
            tenant = None

        try:
            run = await self.go_engine.create_execution_run(tenant_id, user_prompt)
        except GoEngineError as exc:
            yield TraceEvent(type="error", run_id="", message=f"failed to start run: {exc.detail}")
            return

        run_id = run["id"]
        yield TraceEvent(type="run_started", run_id=run_id, message=user_prompt)

        try:
            tools = await self._load_tools(tenant_id)
        except GoEngineError as exc:
            yield TraceEvent(type="error", run_id=run_id, message=f"failed to load tools: {exc.detail}")
            await self.go_engine.update_run_status(run_id, "failed")
            return

        messages = [{"role": "user", "content": [{"type": "text", "text": user_prompt}]}]
        system_prompt = SYSTEM_PROMPT + build_tenant_context(tenant_id)

        async for event in self._drive_loop(run_id, tenant_id, provider, system_prompt, messages, tools):
            yield event

    async def resume(self, tenant_id: str, run_id: str, step_id: str, approved: bool, provider: Optional[str] = None) -> AsyncGenerator[TraceEvent, None]:
        provider = provider or self.settings.default_llm_provider

        try:
            step_result = await self.go_engine.confirm_step(run_id, step_id, approved)
        except GoEngineError as exc:
            yield TraceEvent(type="error", run_id=run_id, step_id=step_id, message=f"failed to confirm step: {exc.detail}")
            return

        if approved:
            yield TraceEvent(
                type="tool_result", run_id=run_id, step_id=step_id,
                tool_name=step_result.get("tool_name"), result=step_result.get("result"),
            )
        else:
            yield TraceEvent(
                type="tool_result", run_id=run_id, step_id=step_id,
                tool_name=step_result.get("tool_name"), message="Step rejected by user - skipped.",
            )

        try:
            run = await self.go_engine.get_execution_run(run_id)
        except GoEngineError as exc:
            yield TraceEvent(type="error", run_id=run_id, message=f"failed to load run: {exc.detail}")
            return

        user_prompt = run["run"]["user_prompt"] if "run" in run else run.get("user_prompt", "")

        try:
            tools = await self._load_tools(tenant_id)
        except GoEngineError as exc:
            yield TraceEvent(type="error", run_id=run_id, message=f"failed to load tools: {exc.detail}")
            return

        messages = await self._rebuild_messages_from_history(run_id, user_prompt)
        system_prompt = SYSTEM_PROMPT + build_tenant_context(tenant_id)

        async for event in self._drive_loop(run_id, tenant_id, provider, system_prompt, messages, tools):
            yield event

    async def _drive_loop(
        self,
        run_id: str,
        tenant_id: str,
        provider: str,
        system_prompt: str,
        messages: list[dict[str, Any]],
        tools: list[ToolDef],
    ) -> AsyncGenerator[TraceEvent, None]:
        tool_defs = self._tool_defs_for_llm(tools)

        for _ in range(MAX_ITERATIONS):
            try:
                response = await call_llm(provider, system_prompt, messages, tool_defs)
            except Exception as exc:
                logger.exception("agent_loop: LLM call failed")
                yield TraceEvent(type="error", run_id=run_id, message=f"LLM call failed: {exc}")
                await self.go_engine.update_run_status(run_id, "failed")
                return

            if response.text:
                yield TraceEvent(type="assistant_message", run_id=run_id, message=response.text)

            if not response.tool_calls:
                yield TraceEvent(type="final_answer", run_id=run_id, message=response.text)
                await self.go_engine.update_run_status(run_id, "completed")
                yield TraceEvent(type="run_completed", run_id=run_id)
                return

            assistant_blocks = [
                {"type": "tool_use", "id": tc.id, "name": tc.name, "input": tc.arguments}
                for tc in response.tool_calls
            ]
            messages.append({"role": "assistant", "content": assistant_blocks})

            paused = False
            tool_result_blocks: list[dict[str, Any]] = []

            for tc in response.tool_calls:
                tool = self._find_tool(tools, tc.name)
                if tool is None:
                    yield TraceEvent(type="error", run_id=run_id, tool_name=tc.name, message=f"unknown tool '{tc.name}'")
                    tool_result_blocks.append({
                        "type": "tool_result", "tool_use_id": tc.id,
                        "content": {"error": f"unknown tool '{tc.name}'"}, "_tool_name": tc.name,
                    })
                    continue

                yield TraceEvent(
                    type="tool_call_proposed", run_id=run_id, tool_name=tool.tool_name,
                    arguments=tc.arguments, is_destructive=tool.is_destructive,
                )

                try:
                    step = await self.go_engine.create_execution_step(
                        run_id, tool.id, tool.tool_name, tc.arguments, tool.is_destructive
                    )
                except GoEngineError as exc:
                    yield TraceEvent(type="error", run_id=run_id, tool_name=tool.tool_name, message=f"failed to record step: {exc.detail}")
                    continue

                step_id = step["id"]

                if tool.is_destructive:
                    yield TraceEvent(
                        type="awaiting_confirmation", run_id=run_id, step_id=step_id,
                        tool_name=tool.tool_name, arguments=tc.arguments, is_destructive=True,
                    )
                    await self.go_engine.update_run_status(run_id, "awaiting_confirmation")
                    paused = True
                    break  # stop processing further tool calls this turn; resume() picks up later

                yield TraceEvent(type="tool_executing", run_id=run_id, step_id=step_id, tool_name=tool.tool_name)

                try:
                    executed = await self.go_engine.execute_step(run_id, step_id)
                except GoEngineError as exc:
                    yield TraceEvent(type="error", run_id=run_id, step_id=step_id, tool_name=tool.tool_name, message=f"execution failed: {exc.detail}")
                    tool_result_blocks.append({
                        "type": "tool_result", "tool_use_id": tc.id,
                        "content": {"error": exc.detail}, "_tool_name": tool.tool_name,
                    })
                    continue

                result = executed.get("result")
                yield TraceEvent(type="tool_result", run_id=run_id, step_id=step_id, tool_name=tool.tool_name, result=result)
                tool_result_blocks.append({
                    "type": "tool_result", "tool_use_id": tc.id, "content": result, "_tool_name": tool.tool_name,
                })

            if paused:
                return  # wait for /plan/confirm to call resume()

            messages.append({"role": "user", "content": tool_result_blocks})

        yield TraceEvent(type="error", run_id=run_id, message="max planning iterations reached without a final answer")
        await self.go_engine.update_run_status(run_id, "failed")