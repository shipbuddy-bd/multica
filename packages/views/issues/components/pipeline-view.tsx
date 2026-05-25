"use client";

import { useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { pipelineEdgesOptions } from "@multica/core/pipelines";
import type {
  Issue,
  PipelineEdge,
  PipelineStage,
  PipelineCheckpointMeta,
} from "@multica/core/types";
import { AppLink } from "../../navigation";
import { BoardEdgeOverlay } from "./board-edge-overlay";
import { EdgeDetailPanel } from "./edge-detail-panel";

const PIPELINE_STAGES: { id: PipelineStage; label: string; color: string }[] = [
  { id: "clarify", label: "Clarify", color: "border-emerald-200 bg-emerald-50/40 dark:border-emerald-900/30 dark:bg-emerald-950/20" },
  { id: "plan", label: "Plan", color: "border-blue-200 bg-blue-50/40 dark:border-blue-900/30 dark:bg-blue-950/20" },
  { id: "implement", label: "Implement", color: "border-violet-200 bg-violet-50/40 dark:border-violet-900/30 dark:bg-violet-950/20" },
  { id: "validate", label: "Validate", color: "border-amber-200 bg-amber-50/40 dark:border-amber-900/30 dark:bg-amber-950/20" },
  { id: "handoff", label: "Handoff", color: "border-rose-200 bg-rose-50/40 dark:border-rose-900/30 dark:bg-rose-950/20" },
];

/**
 * Pipeline View — alternative Board layout that shows the 6-stage pipeline
 * (clarify → plan → implement → validate → handoff) as columns.
 *
 * Issues are placed into columns based on their `metadata.pipeline.stage`
 * field set by the orchestrator. Issues without pipeline metadata are
 * filtered out so this view stays focused on AI-driven requirement delivery.
 *
 * Reuses BoardEdgeOverlay + EdgeDetailPanel for arrow rendering and
 * click-to-inspect behaviour.
 */
export function PipelineView({ issues }: { issues: Issue[] }) {
  const wsId = useWorkspaceId();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [selectedEdge, setSelectedEdge] = useState<PipelineEdge | null>(null);

  const edgesQuery = useQuery({
    ...pipelineEdgesOptions(wsId),
    enabled: !!wsId,
  });
  const edges: PipelineEdge[] = edgesQuery.data ?? [];

  // Group issues by pipeline stage. Anything without metadata.pipeline.stage
  // is excluded so the view stays uncluttered.
  const byStage = useMemo(() => {
    const groups: Record<PipelineStage, Issue[]> = {
      clarify: [],
      plan: [],
      implement: [],
      validate: [],
      handoff: [],
    };
    for (const issue of issues) {
      const meta = extractPipelineMeta(issue);
      if (!meta) continue;
      // Only show checkpoints, not the root issue itself.
      if (meta.role !== "checkpoint") continue;
      const stage = meta.stage;
      if (groups[stage]) {
        groups[stage].push(issue);
      }
    }
    return groups;
  }, [issues]);

  const totalCheckpoints = Object.values(byStage).reduce(
    (sum, list) => sum + list.length,
    0,
  );

  if (totalCheckpoints === 0) {
    return (
      <div className="flex flex-1 items-center justify-center p-8 text-sm text-muted-foreground">
        <div className="text-center">
          <div className="mb-2 font-medium">还没有 Pipeline 数据</div>
          <div className="text-xs">
            通过 Chat 发起一个需求，或在 issue 上触发 pipeline orchestrator 后，
            checkpoints 会按阶段出现在这里。
          </div>
        </div>
      </div>
    );
  }

  return (
    <>
      <div
        ref={containerRef}
        className="relative flex flex-1 min-h-0 gap-4 overflow-x-auto p-4"
      >
        {PIPELINE_STAGES.map((stage) => (
          <PipelineColumn
            key={stage.id}
            stage={stage}
            issues={byStage[stage.id]}
          />
        ))}

        <BoardEdgeOverlay
          edges={edges}
          containerRef={containerRef}
          onEdgeClick={setSelectedEdge}
        />
      </div>

      <EdgeDetailPanel
        edge={selectedEdge}
        onClose={() => setSelectedEdge(null)}
      />
    </>
  );
}

function PipelineColumn({
  stage,
  issues,
}: {
  stage: { id: PipelineStage; label: string; color: string };
  issues: Issue[];
}) {
  return (
    <div
      className={`flex w-[280px] shrink-0 flex-col rounded-xl border bg-muted/40 p-2 ${stage.color}`}
    >
      <div className="mb-2 flex items-center justify-between px-2 py-1.5">
        <span className="text-sm font-semibold capitalize">{stage.label}</span>
        <span className="text-xs text-muted-foreground">{issues.length}</span>
      </div>
      <div className="min-h-[200px] flex-1 space-y-2 overflow-y-auto rounded-lg p-1">
        {issues.length === 0 ? (
          <div className="flex h-20 items-center justify-center text-xs text-muted-foreground/70">
            暂无 checkpoint
          </div>
        ) : (
          issues.map((issue) => (
            <PipelineCard key={issue.id} issue={issue} />
          ))
        )}
      </div>
    </div>
  );
}

function PipelineCard({ issue }: { issue: Issue }) {
  const p = useWorkspacePaths();
  const meta = extractPipelineMeta(issue);
  const score = meta?.score?.total;

  return (
    <AppLink
      href={p.issueDetail(issue.id)}
      data-issue-id={issue.id}
      className="block rounded-lg border-[0.5px] border-border bg-card px-2.5 py-3 transition-colors hover:border-foreground/20"
    >
      <div className="mb-1 text-[11px] text-muted-foreground">
        {issue.identifier ?? `#${issue.number}`}
        {meta?.variant && (
          <span className="ml-1 font-mono">· {meta.variant}</span>
        )}
      </div>
      <div className="line-clamp-2 text-sm font-medium leading-snug">
        {issue.title}
      </div>

      <div className="mt-2 flex items-center gap-2">
        {typeof score === "number" && (
          <span
            className={`inline-flex items-center rounded-md px-1.5 py-0.5 text-[11px] font-semibold ${scoreBadgeColor(score)}`}
          >
            {Math.round(score)}
          </span>
        )}
        {meta?.branch_name && (
          <span className="truncate text-[10px] text-muted-foreground/80 font-mono">
            {meta.branch_name}
          </span>
        )}
      </div>
    </AppLink>
  );
}

function extractPipelineMeta(issue: Issue): PipelineCheckpointMeta | null {
  const md = issue.metadata as Record<string, unknown> | null | undefined;
  if (!md) return null;
  const raw = md.pipeline as PipelineCheckpointMeta | undefined;
  if (!raw || typeof raw !== "object") return null;
  return raw;
}

function scoreBadgeColor(score: number): string {
  if (score >= 80) return "bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400";
  if (score >= 60) return "bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-400";
  return "bg-rose-100 text-rose-700 dark:bg-rose-950/40 dark:text-rose-400";
}
