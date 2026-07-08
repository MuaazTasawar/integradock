"use client";

import clsx from "clsx";
import type { TraceEvent } from "@/types";

const STATUS_STYLES: Record<string, string> = {
  run_started: "border-slate-300 bg-slate-50",
  assistant_message: "border-accent/30 bg-blue-50",
  tool_call_proposed: "border-slate-300 bg-white",
  awaiting_confirmation: "border-danger/40 bg-red-50",
  tool_executing: "border-amber-300 bg-amber-50",
  tool_result: "border-success/40 bg-green-50",
  final_answer: "border-success/40 bg-green-50",
  run_completed: "border-slate-300 bg-slate-50",
  error: "border-danger bg-red-50",
};

function eventLabel(event: TraceEvent): string {
  switch (event.type) {
    case "run_started":
      return "Run started";
    case "assistant_message":
      return "Agent";
    case "tool_call_proposed":
      return `Proposed: ${event.tool_name}`;
    case "awaiting_confirmation":
      return `Awaiting confirmation: ${event.tool_name}`;
    case "tool_executing":
      return `Executing: ${event.tool_name}`;
    case "tool_result":
      return `Result: ${event.tool_name}`;
    case "final_answer":
      return "Final answer";
    case "run_completed":
      return "Run completed";
    case "error":
      return "Error";
    default:
      return event.type;
  }
}

export default function TracePanel({ events }: { events: TraceEvent[] }) {
  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-ink">Live execution trace</h2>
      </div>

      <div className="flex-1 overflow-y-auto scrollbar-thin px-4 py-3 space-y-2">
        {events.length === 0 && (
          <p className="text-xs text-muted">
            Tool calls and results will stream here as the agent works.
          </p>
        )}

        {events.map((event, idx) => (
          <div
            key={`${event.type}-${event.step_id || idx}-${idx}`}
            className={clsx(
              "rounded-lg border px-3 py-2 text-xs",
              STATUS_STYLES[event.type] || "border-slate-200 bg-white"
            )}
          >
            <div className="flex items-center justify-between mb-1">
              <span className="font-semibold text-ink">{eventLabel(event)}</span>
              {event.is_destructive && (
                <span className="text-[10px] uppercase tracking-wide text-danger font-semibold">
                  destructive
                </span>
              )}
            </div>

            {event.message && (
              <p className="text-ink whitespace-pre-wrap">{event.message}</p>
            )}

            {event.arguments && Object.keys(event.arguments).length > 0 && (
              <pre className="mt-1 font-mono text-[11px] text-muted whitespace-pre-wrap break-words">
                {JSON.stringify(event.arguments, null, 2)}
              </pre>
            )}

            {event.result !== undefined && event.result !== null && (
              <pre className="mt-1 font-mono text-[11px] text-ink whitespace-pre-wrap break-words">
                {JSON.stringify(event.result, null, 2)}
              </pre>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}