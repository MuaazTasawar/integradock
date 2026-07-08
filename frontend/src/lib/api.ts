import type { ParsedConnection, RegisteredTool, TraceEvent } from "@/types";

const GO_ENGINE_URL =
  process.env.NEXT_PUBLIC_GO_ENGINE_URL || "http://localhost:8080";
const PY_PLANNER_URL =
  process.env.NEXT_PUBLIC_PY_PLANNER_URL || "http://localhost:8000";
const INTERNAL_SECRET = process.env.NEXT_PUBLIC_INTERNAL_API_SECRET || "";

function goHeaders(extra?: HeadersInit): HeadersInit {
  return {
    "X-Internal-Secret": INTERNAL_SECRET,
    ...(extra || {}),
  };
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function handleJson<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let detail = res.statusText;
    try {
      const body = await res.json();
      detail = body.error || detail;
    } catch {
      // response wasn't JSON, keep statusText
    }
    throw new ApiError(res.status, detail);
  }
  return res.json() as Promise<T>;
}

// ---- Tenants ----

export interface Tenant {
  id: string;
  name: string;
  slug: string;
}

export async function createTenant(name: string, slug: string): Promise<Tenant> {
  const res = await fetch(`${GO_ENGINE_URL}/api/tenants/`, {
    method: "POST",
    headers: goHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify({ name, slug }),
  });
  return handleJson<Tenant>(res);
}

export async function getTenantBySlug(slug: string): Promise<Tenant> {
  const res = await fetch(`${GO_ENGINE_URL}/api/tenants/${slug}`, {
    headers: goHeaders(),
  });
  return handleJson<Tenant>(res);
}

// ---- Spec parsing + tool registration ----

export async function parseSpecUpload(params: {
  file: File;
  connectionName: string;
  baseUrl?: string;
  authType: string;
  authConfig: Record<string, unknown>;
}): Promise<ParsedConnection> {
  const form = new FormData();
  form.append("file", params.file);
  form.append("connection_name", params.connectionName);
  if (params.baseUrl) form.append("base_url", params.baseUrl);
  form.append("auth_type", params.authType);
  form.append("auth_config", JSON.stringify(params.authConfig));

  const res = await fetch(`${PY_PLANNER_URL}/parse/upload`, {
    method: "POST",
    body: form,
  });
  return handleJson<ParsedConnection>(res);
}

export async function registerConnection(
  tenantId: string,
  parsed: ParsedConnection
): Promise<{ connection: unknown; tools: RegisteredTool[] }> {
  const res = await fetch(`${GO_ENGINE_URL}/api/tools/connections`, {
    method: "POST",
    headers: goHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify({
      tenant_id: tenantId,
      name: parsed.name,
      base_url: parsed.base_url,
      auth_type: parsed.auth_type,
      auth_config: parsed.auth_config,
      spec_raw: parsed.spec_raw,
      tools: parsed.tools,
    }),
  });
  return handleJson(res);
}

export async function listTools(tenantId: string): Promise<RegisteredTool[]> {
  const res = await fetch(
    `${GO_ENGINE_URL}/api/tools?tenant_id=${encodeURIComponent(tenantId)}`,
    { headers: goHeaders() }
  );
  return handleJson<RegisteredTool[]>(res);
}

// ---- Planning (SSE) ----

/**
 * Reads a fetch Response's body as newline-delimited SSE frames
 * ("data: {...}\n\n") and invokes onEvent for each parsed TraceEvent.
 * Used for both /plan/start and /plan/confirm since EventSource can't
 * send POST bodies.
 */
async function consumeSSE(
  res: Response,
  onEvent: (event: TraceEvent) => void
): Promise<void> {
  if (!res.body) {
    throw new Error("api: response has no readable body for SSE stream");
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const frames = buffer.split("\n\n");
    buffer = frames.pop() || "";

    for (const frame of frames) {
      const line = frame.trim();
      if (!line.startsWith("data:")) continue;
      const jsonStr = line.slice("data:".length).trim();
      if (!jsonStr) continue;
      try {
        const event = JSON.parse(jsonStr) as TraceEvent;
        onEvent(event);
      } catch (err) {
        console.error("api: failed to parse SSE frame", err, jsonStr);
      }
    }
  }
}

export async function startPlan(
  tenantId: string,
  userPrompt: string,
  onEvent: (event: TraceEvent) => void
): Promise<void> {
  const res = await fetch(`${PY_PLANNER_URL}/plan/start`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tenant_id: tenantId, user_prompt: userPrompt }),
  });
  if (!res.ok) {
    throw new ApiError(res.status, `failed to start plan: ${res.statusText}`);
  }
  await consumeSSE(res, onEvent);
}

export async function confirmStep(
  tenantId: string,
  runId: string,
  stepId: string,
  approved: boolean,
  onEvent: (event: TraceEvent) => void
): Promise<void> {
  const res = await fetch(`${PY_PLANNER_URL}/plan/confirm`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenant_id: tenantId,
      run_id: runId,
      step_id: stepId,
      approved,
    }),
  });
  if (!res.ok) {
    throw new ApiError(res.status, `failed to confirm step: ${res.statusText}`);
  }
  await consumeSSE(res, onEvent);
}