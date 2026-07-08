"use client";

import { useState } from "react";
import { parseSpecUpload, registerConnection } from "@/lib/api";
import type { ParsedConnection } from "@/types";

interface Props {
  tenantId: string;
  onRegistered: (toolCount: number) => void;
}

export default function ToolUploader({ tenantId, onRegistered }: Props) {
  const [file, setFile] = useState<File | null>(null);
  const [connectionName, setConnectionName] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [authType, setAuthType] = useState("none");
  const [authValue, setAuthValue] = useState("");
  const [status, setStatus] = useState<"idle" | "parsing" | "registering" | "done" | "error">("idle");
  const [errorMsg, setErrorMsg] = useState("");
  const [preview, setPreview] = useState<ParsedConnection | null>(null);

  function buildAuthConfig(): Record<string, unknown> {
    if (authType === "bearer") return { token: authValue };
    if (authType === "api_key") return { header: "X-API-Key", value: authValue };
    if (authType === "basic") {
      const [username, password] = authValue.split(":");
      return { username, password };
    }
    return {};
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!file || !connectionName || !tenantId) return;

    setStatus("parsing");
    setErrorMsg("");
    try {
      const parsed = await parseSpecUpload({
        file,
        connectionName,
        baseUrl: baseUrl || undefined,
        authType,
        authConfig: buildAuthConfig(),
      });
      setPreview(parsed);

      setStatus("registering");
      await registerConnection(tenantId, parsed);

      setStatus("done");
      onRegistered(parsed.tools.length);
    } catch (err) {
      setStatus("error");
      setErrorMsg(err instanceof Error ? err.message : "unknown error");
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3 rounded-xl border border-slate-200 bg-white p-4">
      <h3 className="text-sm font-semibold text-ink">Connect an API</h3>

      <div>
        <label className="block text-xs text-muted mb-1">OpenAPI spec file (.json / .yaml)</label>
        <input
          type="file"
          accept=".json,.yaml,.yml"
          onChange={(e) => setFile(e.target.files?.[0] || null)}
          className="w-full text-sm"
          required
        />
      </div>

      <div>
        <label className="block text-xs text-muted mb-1">Connection name</label>
        <input
          type="text"
          value={connectionName}
          onChange={(e) => setConnectionName(e.target.value)}
          placeholder="e.g. Mock Inventory API"
          className="w-full rounded-lg border border-slate-300 px-3 py-1.5 text-sm"
          required
        />
      </div>

      <div>
        <label className="block text-xs text-muted mb-1">Base URL override (optional)</label>
        <input
          type="text"
          value={baseUrl}
          onChange={(e) => setBaseUrl(e.target.value)}
          placeholder="http://localhost:9090"
          className="w-full rounded-lg border border-slate-300 px-3 py-1.5 text-sm"
        />
      </div>

      <div className="grid grid-cols-2 gap-2">
        <div>
          <label className="block text-xs text-muted mb-1">Auth type</label>
          <select
            value={authType}
            onChange={(e) => setAuthType(e.target.value)}
            className="w-full rounded-lg border border-slate-300 px-3 py-1.5 text-sm"
          >
            <option value="none">None</option>
            <option value="bearer">Bearer token</option>
            <option value="api_key">API key header</option>
            <option value="basic">Basic (user:pass)</option>
          </select>
        </div>
        {authType !== "none" && (
          <div>
            <label className="block text-xs text-muted mb-1">
              {authType === "basic" ? "username:password" : "value"}
            </label>
            <input
              type="text"
              value={authValue}
              onChange={(e) => setAuthValue(e.target.value)}
              className="w-full rounded-lg border border-slate-300 px-3 py-1.5 text-sm"
            />
          </div>
        )}
      </div>

      <button
        type="submit"
        disabled={status === "parsing" || status === "registering"}
        className="w-full rounded-lg bg-accent text-white text-sm font-medium py-2 disabled:opacity-50"
      >
        {status === "parsing" && "Parsing spec..."}
        {status === "registering" && "Registering tools..."}
        {(status === "idle" || status === "done" || status === "error") && "Parse & Register Tools"}
      </button>

      {status === "done" && preview && (
        <p className="text-xs text-success">
          Registered {preview.tools.length} tool(s) from {preview.name}.
        </p>
      )}
      {status === "error" && <p className="text-xs text-danger">{errorMsg}</p>}
    </form>
  );
}