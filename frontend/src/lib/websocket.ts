import type { StepEvent } from "@/types";

const WS_URL = process.env.NEXT_PUBLIC_WS_URL || "ws://localhost:8080/ws";

/**
 * Opens a websocket subscription to a run's live execution trace
 * (go-engine's /ws/executions/:run_id, backed by Redis pub/sub).
 * Returns a close() function; call it on unmount / run completion.
 */
export function connectExecutionSocket(
  runId: string,
  onEvent: (event: StepEvent) => void,
  onClose?: () => void
): () => void {
  const socket = new WebSocket(`${WS_URL}/executions/${runId}`);

  socket.onmessage = (msg) => {
    try {
      const event = JSON.parse(msg.data) as StepEvent;
      onEvent(event);
    } catch (err) {
      console.error("websocket: failed to parse StepEvent", err, msg.data);
    }
  };

  socket.onerror = (err) => {
    console.error("websocket: connection error", err);
  };

  socket.onclose = () => {
    onClose?.();
  };

  return () => {
    if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) {
      socket.close();
    }
  };
}