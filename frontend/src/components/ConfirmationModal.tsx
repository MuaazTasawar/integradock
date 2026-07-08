import type { PendingConfirmation } from "@/types";

interface Props {
  confirmation: PendingConfirmation;
  onApprove: () => void;
  onReject: () => void;
  isProcessing: boolean;
}

export default function ConfirmationModal({
  confirmation,
  onApprove,
  onReject,
  isProcessing,
}: Props) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 px-4">
      <div className="w-full max-w-md rounded-xl bg-white p-6 shadow-xl border border-slate-200">
        <div className="flex items-center gap-2 mb-3">
          <span className="inline-flex h-2.5 w-2.5 rounded-full bg-danger" />
          <h2 className="text-base font-semibold text-ink">
            Confirmation required
          </h2>
        </div>

        <p className="text-sm text-muted mb-4">
          The agent wants to run a destructive action. Review the details
          below before allowing it to proceed.
        </p>

        <div className="rounded-lg bg-slate-50 border border-slate-200 p-3 mb-5 font-mono text-xs">
          <div className="mb-1">
            <span className="text-muted">tool: </span>
            <span className="text-ink font-semibold">
              {confirmation.toolName}
            </span>
          </div>
          <pre className="whitespace-pre-wrap break-words text-ink">
            {JSON.stringify(confirmation.arguments, null, 2)}
          </pre>
        </div>

        <div className="flex gap-3 justify-end">
          <button
            onClick={onReject}
            disabled={isProcessing}
            className="px-4 py-2 rounded-lg text-sm font-medium border border-slate-300 text-ink hover:bg-slate-50 disabled:opacity-50"
          >
            Reject
          </button>
          <button
            onClick={onApprove}
            disabled={isProcessing}
            className="px-4 py-2 rounded-lg text-sm font-medium bg-danger text-white hover:bg-red-700 disabled:opacity-50"
          >
            {isProcessing ? "Running..." : "Approve & Run"}
          </button>
        </div>
      </div>
    </div>
  );
}