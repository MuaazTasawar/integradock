"use client";

import { useState } from "react";
import { confirmStep, startPlan } from "@/lib/api";
import type { ChatMessage, PendingConfirmation, TraceEvent } from "@/types";
import MessageBubble from "./MessageBubble";
import ConfirmationModal from "./ConfirmationModal";

interface Props {
  tenantId: string;
  onTraceEvent: (event: TraceEvent) => void;
}

export default function ChatPanel({ tenantId, onTraceEvent }: Props) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [isRunning, setIsRunning] = useState(false);
  const [runId, setRunId] = useState<string | null>(null);
  const [pendingConfirmation, setPendingConfirmation] =
    useState<PendingConfirmation | null>(null);
  const [isConfirming, setIsConfirming] = useState(false);

  function appendMessage(role: ChatMessage["role"], content: string) {
    setMessages((prev) => [
      ...prev,
      { id: `${Date.now()}-${Math.random()}`, role, content },
    ]);
  }

  function handleEvent(event: TraceEvent) {
    onTraceEvent(event);

    if (event.type === "run_started") {
      setRunId(event.run_id);
    } else if (event.type === "assistant_message" && event.message) {
      appendMessage("assistant", event.message);
    } else if (event.type === "awaiting_confirmation" && event.step_id) {
      setPendingConfirmation({
        runId: event.run_id,
        stepId: event.step_id,
        toolName: event.tool_name || "unknown_tool",
        arguments: event.arguments || {},
      });
    } else if (event.type === "final_answer" && event.message) {
      appendMessage("assistant", event.message);
    } else if (event.type === "error" && event.message) {
      appendMessage("system", `Error: ${event.message}`);
    }
  }

  async function handleSend(e: React.FormEvent) {
    e.preventDefault();
    const prompt = input.trim();
    if (!prompt || !tenantId || isRunning) return;

    appendMessage("user", prompt);
    setInput("");
    setIsRunning(true);

    try {
      await startPlan(tenantId, prompt, handleEvent);
    } catch (err) {
      appendMessage(
        "system",
        `Error: ${err instanceof Error ? err.message : "failed to reach planner"}`
      );
    } finally {
      setIsRunning(false);
    }
  }

  async function handleConfirmation(approved: boolean) {
    if (!pendingConfirmation) return;
    setIsConfirming(true);
    const { runId: rid, stepId } = pendingConfirmation;

    try {
      await confirmStep(tenantId, rid, stepId, approved, handleEvent);
    } catch (err) {
      appendMessage(
        "system",
        `Error: ${err instanceof Error ? err.message : "failed to confirm step"}`
      );
    } finally {
      setPendingConfirmation(null);
      setIsConfirming(false);
    }
  }

  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-ink">Chat</h2>
        {runId && (
          <p className="text-[11px] text-muted font-mono mt-0.5">run: {runId}</p>
        )}
      </div>

      <div className="flex-1 overflow-y-auto scrollbar-thin px-4 py-3 space-y-3">
        {messages.length === 0 && (
          <p className="text-xs text-muted">
            Type a plain-English request, e.g. "place an order for 3 wireless mice".
          </p>
        )}
        {messages.map((m) => (
          <MessageBubble key={m.id} message={m} />
        ))}
      </div>

      <form onSubmit={handleSend} className="border-t border-slate-200 p-3 flex gap-2">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Ask the agent to do something..."
          disabled={isRunning || !tenantId}
          className="flex-1 rounded-lg border border-slate-300 px-3 py-2 text-sm disabled:bg-slate-50"
        />
        <button
          type="submit"
          disabled={isRunning || !tenantId || !input.trim()}
          className="rounded-lg bg-accent text-white text-sm font-medium px-4 py-2 disabled:opacity-50"
        >
          {isRunning ? "Working..." : "Send"}
        </button>
      </form>

      {pendingConfirmation && (
        <ConfirmationModal
          confirmation={pendingConfirmation}
          isProcessing={isConfirming}
          onApprove={() => handleConfirmation(true)}
          onReject={() => handleConfirmation(false)}
        />
      )}
    </div>
  );
}