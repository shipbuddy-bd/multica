import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  pipelineAnnotationsOptions,
  useCreatePipelineAnnotation,
} from "@multica/core/pipelines";
import type {
  PipelineEdge,
  PipelineAnnotation,
} from "@multica/core/types";
import { parseEdgeMetadata } from "@multica/core/types";
import { Sheet, SheetContent } from "@multica/ui/components/ui/sheet";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Badge } from "@multica/ui/components/ui/badge";

interface EdgeDetailPanelProps {
  edge: PipelineEdge | null;
  onClose: () => void;
}

/**
 * Side panel that opens when the user clicks an edge on the board.
 * Shows transition info (source → target stage), score breakdown,
 * AI metrics (tokens / time), branch info, and lets the user add
 * annotations.
 */
export function EdgeDetailPanel({ edge, onClose }: EdgeDetailPanelProps) {
  const open = edge !== null;
  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent
        side="right"
        className="w-[420px] overflow-y-auto p-0 sm:max-w-[420px]"
      >
        {edge && <EdgeDetailContent edge={edge} />}
      </SheetContent>
    </Sheet>
  );
}

function EdgeDetailContent({ edge }: { edge: PipelineEdge }) {
  const meta = parseEdgeMetadata(edge.metadata);
  const score = meta.score;
  const transition = meta.transition ?? `${edge.edge_type} edge`;

  const annotationsQuery = useQuery(pipelineAnnotationsOptions(edge.id));
  const annotations = annotationsQuery.data ?? [];

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="border-b px-5 py-4">
        <div className="flex items-center gap-2">
          <Badge variant="outline" className={edgeBadgeClass(edge.edge_type)}>
            {edge.edge_type}
          </Badge>
          <span className="text-sm font-semibold">{transition}</span>
        </div>
        {meta.variant && (
          <div className="mt-1 text-xs text-muted-foreground">
            Variant: <span className="font-mono">{meta.variant}</span>
          </div>
        )}
        {meta.branch && (
          <div className="mt-1 text-xs text-muted-foreground">
            Branch: <span className="font-mono">{meta.branch}</span>
          </div>
        )}
      </div>

      {/* Score breakdown */}
      {score && (
        <div className="border-b px-5 py-4">
          <div className="mb-3 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Score
          </div>
          <div className="mb-2 flex items-baseline gap-2">
            <span className="text-3xl font-semibold">
              {Math.round(score.total)}
            </span>
            <span className="text-sm text-muted-foreground">/ 100</span>
          </div>
          <div className="space-y-1.5 text-xs">
            <ScoreRow
              label="Lint"
              value={score.lint_pass ? "PASS" : "FAIL"}
              tone={score.lint_pass ? "good" : "bad"}
            />
            {typeof score.test_pass_rate === "number" && (
              <ScoreRow
                label="Tests"
                value={`${score.test_passed ?? 0}/${score.test_total ?? 0} (${Math.round(
                  score.test_pass_rate * 100,
                )}%)`}
                tone={score.test_pass_rate >= 0.9 ? "good" : "neutral"}
              />
            )}
            {typeof score.token_consumed === "number" && (
              <ScoreRow
                label="Tokens"
                value={score.token_consumed.toLocaleString()}
              />
            )}
            {typeof score.generation_time_ms === "number" && (
              <ScoreRow
                label="Duration"
                value={formatDuration(score.generation_time_ms)}
              />
            )}
            {typeof score.diff_lines_added === "number" && (
              <ScoreRow
                label="Diff"
                value={`+${score.diff_lines_added} / -${score.diff_lines_removed ?? 0} (${score.diff_files_changed ?? 0} files)`}
              />
            )}
          </div>
        </div>
      )}

      {/* Annotations */}
      <div className="flex-1 overflow-y-auto px-5 py-4">
        <div className="mb-3 text-xs font-medium uppercase tracking-wide text-muted-foreground">
          Annotations ({annotations.length})
        </div>
        <AnnotationList annotations={annotations} />
        <AnnotationComposer edgeId={edge.id} />
      </div>
    </div>
  );
}

function ScoreRow({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: "good" | "bad" | "neutral";
}) {
  const toneClass =
    tone === "good"
      ? "text-emerald-600 dark:text-emerald-400"
      : tone === "bad"
        ? "text-red-600 dark:text-red-400"
        : "text-foreground";
  return (
    <div className="flex items-center justify-between">
      <span className="text-muted-foreground">{label}</span>
      <span className={`font-mono ${toneClass}`}>{value}</span>
    </div>
  );
}

function AnnotationList({
  annotations,
}: {
  annotations: PipelineAnnotation[];
}) {
  if (annotations.length === 0) {
    return (
      <div className="mb-4 text-xs text-muted-foreground italic">
        No annotations yet.
      </div>
    );
  }
  return (
    <div className="mb-4 space-y-2">
      {annotations.map((a) => (
        <div
          key={a.id}
          className="rounded-md border bg-muted/40 px-3 py-2 text-sm"
        >
          <div className="mb-1 flex items-center gap-2">
            <Badge variant="outline" className="text-[10px]">
              {a.annotation_type}
            </Badge>
            <span className="text-xs text-muted-foreground">
              {new Date(a.created_at).toLocaleString()}
            </span>
          </div>
          <div className="whitespace-pre-wrap break-words">{a.content}</div>
        </div>
      ))}
    </div>
  );
}

function AnnotationComposer({ edgeId }: { edgeId: string }) {
  const [content, setContent] = useState("");
  const mut = useCreatePipelineAnnotation(edgeId);

  const handleSubmit = () => {
    if (!content.trim()) return;
    mut.mutate(
      { content: content.trim(), annotation_type: "note" },
      {
        onSuccess: () => setContent(""),
      },
    );
  };

  return (
    <div className="space-y-2">
      <Textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder="Add a note about this transition..."
        className="min-h-[80px] resize-none text-sm"
      />
      <div className="flex justify-end">
        <Button
          size="sm"
          onClick={handleSubmit}
          disabled={!content.trim() || mut.isPending}
        >
          {mut.isPending ? "Adding..." : "Add note"}
        </Button>
      </div>
    </div>
  );
}

function edgeBadgeClass(edgeType: string): string {
  switch (edgeType) {
    case "branch":
      return "border-blue-500/50 text-blue-600 dark:text-blue-400";
    case "merge":
      return "border-purple-500/50 text-purple-600 dark:text-purple-400";
    case "retry":
      return "border-amber-500/50 text-amber-600 dark:text-amber-400";
    case "flow":
    default:
      return "border-emerald-500/50 text-emerald-600 dark:text-emerald-400";
  }
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60_000).toFixed(1)}m`;
}
