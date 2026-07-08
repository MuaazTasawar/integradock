"use client";

import { useState } from "react";
import ChatPanel from "@/components/ChatPanel";
import TracePanel from "@/components/TracePanel";
import ToolUploader from "@/components/ToolUploader";
import type { TraceEvent } from "@/types";

export default function HomePage() {
  const [tenantId, setTenantId] = useState("");
  const [tenantIdInput, setTenantIdInput] = useState("");
  const [toolCount, setToolCount] = useState<number | null>(null);
  const [events, setEvents] = useState<TraceEvent[]>([]);

  function handleSetTenant(e: React.FormEvent) {
    e.preventDefault();
    if (tenantIdInput.trim()) setTenantId(tenantIdInput.trim());
  }

  return (
    <main className="h-screen flex flex-col">
      <header className="border-b border-slate-200 px-6 py-4 flex items-center justify-between bg-white">
        <div>
          <h1 className="text-lg font-semibold text-ink">IntegraDock</h1>
          <p className="text-xs text-muted">
            OpenAPI specs -&gt; live, callable AI agent tools
          </p>
        </div>

        <form onSubmit={handleSetTenant} className="flex items-center gap-2">
          <input
            type="text"
            value={tenantIdInput}
            onChange={(e) => setTenantIdInput(e.target.value)}
            placeholder="Tenant ID (UUID)"
            className="rounded-lg border border-slate-300 px-3 py-1.5 text-xs w-56 font-mono"
          />
          <button
            type="submit"
            className="rounded-lg bg-ink text-white text-xs font-medium px-3 py-1.5"
          >
            Set tenant
          </button>
        </form>
      </header>

      <div className="flex-1 grid grid-cols-[280px_1fr_360px] overflow-hidden">
        <aside className="border-r border-slate-200 bg-slate-50 overflow-y-auto scrollbar-thin p-4 space-y-4">
          {tenantId ? (
            <ToolUploader tenantId={tenantId} onRegistered={setToolCount} />
          ) : (
            <p className="text-xs text-muted">
              Set a tenant ID above to connect an API.
            </p>
          )}
          {toolCount !== null && (
            <p className="text-xs text-success">{toolCount} tool(s) available to the agent.</p>
          )}
        </aside>

        <section className="border-r border-slate-200 bg-white overflow-hidden">
          <ChatPanel
            tenantId={tenantId}
            onTraceEvent={(event) => setEvents((prev) => [...prev, event])}
          />
        </section>

        <aside className="bg-white overflow-hidden">
          <TracePanel events={events} />
        </aside>
      </div>
    </main>
  );
}