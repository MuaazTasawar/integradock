"""
POST /plan/start   - kicks off a new agent run, streams TraceEvents over SSE
POST /plan/confirm - approves/rejects a destructive step, streams the rest of the run
"""
import json
from typing import AsyncGenerator

from fastapi import APIRouter
from fastapi.responses import StreamingResponse

from app.planner.agent_loop import AgentLoop
from app.planner.models import PlanConfirmRequest, PlanStartRequest

router = APIRouter(prefix="/plan", tags=["plan"])


async def _stream(generator) -> AsyncGenerator[str, None]:
    async for event in generator:
        yield event.to_sse()


@router.post("/start")
async def start_plan(req: PlanStartRequest) -> StreamingResponse:
    loop = AgentLoop()
    generator = loop.start(req.tenant_id, req.user_prompt, req.llm_provider)
    return StreamingResponse(_stream(generator), media_type="text/event-stream")


@router.post("/confirm")
async def confirm_step(req: PlanConfirmRequest) -> StreamingResponse:
    loop = AgentLoop()
    generator = loop.resume(req.tenant_id, req.run_id, req.step_id, req.approved, req.llm_provider)
    return StreamingResponse(_stream(generator), media_type="text/event-stream")