"""
Async HTTP client for go-engine's internal API. All calls carry the
X-Internal-Secret header expected by go-engine's InternalAuth middleware.
"""
import logging
from typing import Any, Optional

import httpx

from app.config import get_settings

logger = logging.getLogger("integradock.go_engine_client")


class GoEngineError(Exception):
    def __init__(self, status_code: int, detail: str):
        self.status_code = status_code
        self.detail = detail
        super().__init__(f"go-engine error {status_code}: {detail}")


class GoEngineClient:
    def __init__(self) -> None:
        settings = get_settings()
        self.base_url = settings.go_engine_url.rstrip("/")
        self.headers = {"X-Internal-Secret": settings.internal_api_secret}

    async def _request(self, method: str, path: str, **kwargs: Any) -> dict[str, Any]:
        async with httpx.AsyncClient(base_url=self.base_url, headers=self.headers, timeout=20.0) as client:
            resp = await client.request(method, path, **kwargs)
            if resp.status_code >= 400:
                try:
                    detail = resp.json().get("error", resp.text)
                except Exception:
                    detail = resp.text
                raise GoEngineError(resp.status_code, detail)
            if resp.status_code == 204 or not resp.content:
                return {}
            return resp.json()

    # ---- Tenants ----

    async def get_tenant(self, slug: str) -> dict[str, Any]:
        return await self._request("GET", f"/api/tenants/{slug}")

    # ---- Tools ----

    async def list_tools(self, tenant_id: str) -> list[dict[str, Any]]:
        result = await self._request("GET", "/api/tools", params={"tenant_id": tenant_id})
        return result if isinstance(result, list) else []

    # ---- Executions ----

    async def create_execution_run(self, tenant_id: str, user_prompt: str) -> dict[str, Any]:
        return await self._request(
            "POST", "/api/executions", json={"tenant_id": tenant_id, "user_prompt": user_prompt}
        )

    async def get_execution_run(self, run_id: str) -> dict[str, Any]:
        return await self._request("GET", f"/api/executions/{run_id}")

    async def update_run_status(self, run_id: str, status: str) -> dict[str, Any]:
        return await self._request("PATCH", f"/api/executions/{run_id}", json={"status": status})

    async def create_execution_step(
        self,
        run_id: str,
        tool_id: Optional[str],
        tool_name: str,
        arguments: dict[str, Any],
        requires_confirmation: bool,
    ) -> dict[str, Any]:
        return await self._request(
            "POST",
            f"/api/executions/{run_id}/steps",
            json={
                "tool_id": tool_id,
                "tool_name": tool_name,
                "arguments": arguments,
                "requires_confirmation": requires_confirmation,
            },
        )

    async def execute_step(self, run_id: str, step_id: str) -> dict[str, Any]:
        """Executes a non-destructive step immediately."""
        return await self._request("POST", f"/api/executions/{run_id}/steps/{step_id}/execute")

    async def confirm_step(self, run_id: str, step_id: str, approved: bool) -> dict[str, Any]:
        """Executes (or skips) a destructive step after human confirmation."""
        return await self._request(
            "POST", f"/api/executions/{run_id}/steps/{step_id}/confirm", json={"approved": approved}
        )