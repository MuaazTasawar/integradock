"""
IntegraDock py-planner service entrypoint.

Responsibilities (grown across phases):
  Phase 4 - OpenAPI spec parsing (/parse/*)
  Phase 5 - LLM planning loop (/plan/*)
"""
import logging

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.config import get_settings
from app.routers import parse, plan

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("integradock.py-planner")

settings = get_settings()

app = FastAPI(
    title="IntegraDock Planner Service",
    description="OpenAPI spec parsing + LLM planning loop for IntegraDock agents",
    version="0.1.0",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:3000"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(parse.router)
app.include_router(plan.router)


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "service": "integradock-py-planner", "env": settings.env}