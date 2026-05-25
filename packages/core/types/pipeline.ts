// Pipeline types for the "Super Individual" requirement delivery system.
// Each pipeline is a DAG of sub-issues (checkpoints) connected by edges.

/** Stages in the pipeline lifecycle. */
export type PipelineStage =
  | "clarify"
  | "plan"
  | "implement"
  | "validate"
  | "handoff";

/** Edge type — direction of a transition in the DAG. */
export type PipelineEdgeType = "flow" | "branch" | "merge" | "retry";

/** Annotation type — kind of user note attached to an edge. */
export type PipelineAnnotationType =
  | "note"
  | "approval"
  | "rejection"
  | "suggestion";

/** Score breakdown for a pipeline checkpoint. */
export interface PipelineScore {
  total: number;
  lint_pass: boolean;
  lint_errors?: number;
  test_total?: number;
  test_passed?: number;
  test_pass_rate?: number;
  token_consumed?: number;
  generation_time_ms?: number;
  diff_lines_added?: number;
  diff_lines_removed?: number;
  diff_files_changed?: number;
}

/** Edge metadata — extra info about a transition (typically score or transition label). */
export interface PipelineEdgeMetadata {
  transition?: string;
  variant?: string;
  branch?: string;
  score?: PipelineScore;
  [key: string]: unknown;
}

/** A pipeline edge connecting two sub-issues. */
export interface PipelineEdge {
  id: string;
  workspace_id: string;
  source_issue_id: string;
  target_issue_id: string;
  edge_type: PipelineEdgeType;
  /** JSONB metadata. The server may return this as an object or a base64 string
   *  depending on serialization; clients should normalize via `parseEdgeMetadata`. */
  metadata: PipelineEdgeMetadata | string;
  created_at: string;
}

/** A user annotation attached to an edge. */
export interface PipelineAnnotation {
  id: string;
  edge_id: string;
  author_id: string;
  content: string;
  annotation_type: PipelineAnnotationType;
  created_at: string;
}

/** Issue-side metadata for a pipeline checkpoint, stored in issue.metadata.pipeline. */
export interface PipelineCheckpointMeta {
  role: "root" | "checkpoint";
  stage: PipelineStage;
  variant?: string;
  branch_name?: string;
  score?: PipelineScore;
  /** Only set on root issues. */
  repo_url?: string;
  base_branch?: string;
}

/** Full DAG response — issues + edges for a pipeline. */
export interface PipelineDAG {
  issues: import("./issue").Issue[];
  edges: PipelineEdge[];
}

/** Request bodies. */
export interface CreatePipelineEdgeRequest {
  source_issue_id: string;
  target_issue_id: string;
  edge_type?: PipelineEdgeType;
  metadata?: PipelineEdgeMetadata;
}

export interface CreatePipelineAnnotationRequest {
  content: string;
  annotation_type?: PipelineAnnotationType;
}

/**
 * Decode the metadata field which the server returns as either:
 *  - a parsed object (preferred path; used by current PipelineEdgeResponse)
 *  - a base64 string (legacy path when the raw []byte JSONB was serialized)
 *
 * The base64 fallback decodes UTF-8 properly, so multi-byte chars (e.g. →,
 * 中文) survive the round trip.
 *
 * Returns an empty object on failure.
 */
export function parseEdgeMetadata(
  raw: PipelineEdgeMetadata | string | null | undefined,
): PipelineEdgeMetadata {
  if (!raw) return {};
  if (typeof raw === "object") return raw;
  try {
    return JSON.parse(raw) as PipelineEdgeMetadata;
  } catch {
    try {
      // Convert base64 → bytes → UTF-8 string. atob alone returns Latin-1
      // and corrupts multi-byte characters like → (E2 86 92), so we go
      // through Uint8Array + TextDecoder for proper UTF-8.
      const bytes = Uint8Array.from(atob(raw), (c) => c.charCodeAt(0));
      const decoded = new TextDecoder("utf-8").decode(bytes);
      return JSON.parse(decoded) as PipelineEdgeMetadata;
    } catch {
      return {};
    }
  }
}
