import type { ChatMessage } from "@/types";
import clsx from "clsx";

export default function MessageBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === "user";
  const isSystem = message.role === "system";

  return (
    <div className={clsx("flex w-full", isUser ? "justify-end" : "justify-start")}>
      <div
        className={clsx(
          "max-w-[80%] rounded-2xl px-4 py-2 text-sm leading-relaxed whitespace-pre-wrap",
          isUser && "bg-accent text-white rounded-br-sm",
          !isUser && !isSystem && "bg-white border border-slate-200 text-ink rounded-bl-sm",
          isSystem && "bg-slate-100 text-muted italic text-xs"
        )}
      >
        {message.content}
      </div>
    </div>
  );
}