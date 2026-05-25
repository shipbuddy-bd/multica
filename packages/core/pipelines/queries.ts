import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type {
  CreatePipelineAnnotationRequest,
  CreatePipelineEdgeRequest,
} from "../types";

/** TanStack Query keys for pipeline data. */
export const pipelineKeys = {
  all: (wsId: string) => ["pipelines", wsId] as const,
  edges: (wsId: string) => [...pipelineKeys.all(wsId), "edges"] as const,
  edgesByParent: (wsId: string, parentIssueId: string) =>
    [...pipelineKeys.edges(wsId), "by-parent", parentIssueId] as const,
  dag: (wsId: string, issueId: string) =>
    [...pipelineKeys.all(wsId), "dag", issueId] as const,
  annotations: (edgeId: string) =>
    ["pipelines", "annotations", edgeId] as const,
};

/** All pipeline edges in a workspace, optionally filtered by parent issue. */
export function pipelineEdgesOptions(
  wsId: string,
  parentIssueId?: string,
) {
  return queryOptions({
    queryKey: parentIssueId
      ? pipelineKeys.edgesByParent(wsId, parentIssueId)
      : pipelineKeys.edges(wsId),
    queryFn: () => api.listPipelineEdges(parentIssueId),
    staleTime: 30_000,
  });
}

/** Full DAG (issues + edges) for a pipeline rooted at the given issue. */
export function pipelineDAGOptions(wsId: string, issueId: string) {
  return queryOptions({
    queryKey: pipelineKeys.dag(wsId, issueId),
    queryFn: () => api.getPipelineDAG(issueId),
    staleTime: 15_000,
  });
}

/** Annotations attached to a single edge. */
export function pipelineAnnotationsOptions(edgeId: string) {
  return queryOptions({
    queryKey: pipelineKeys.annotations(edgeId),
    queryFn: () => api.listPipelineAnnotations(edgeId),
    staleTime: 30_000,
  });
}

/** Mutation: create a pipeline edge (rarely called from the UI;
 *  edges are usually created by the orchestrator). */
export function useCreatePipelineEdge(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreatePipelineEdgeRequest) =>
      api.createPipelineEdge(data),
    onSuccess: (_edge, vars) => {
      qc.invalidateQueries({
        queryKey: pipelineKeys.edges(wsId),
      });
      // Source issue's DAG view also needs refresh
      qc.invalidateQueries({
        queryKey: pipelineKeys.dag(wsId, vars.source_issue_id),
      });
    },
  });
}

/** Mutation: add an annotation to an edge. */
export function useCreatePipelineAnnotation(edgeId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreatePipelineAnnotationRequest) =>
      api.createPipelineAnnotation(edgeId, data),
    onSuccess: () => {
      qc.invalidateQueries({
        queryKey: pipelineKeys.annotations(edgeId),
      });
    },
  });
}
