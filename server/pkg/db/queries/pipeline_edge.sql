-- Pipeline Edge CRUD

-- name: CreatePipelineEdge :one
INSERT INTO pipeline_edge (workspace_id, source_issue_id, target_issue_id, edge_type, metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPipelineEdge :one
SELECT * FROM pipeline_edge
WHERE id = $1;

-- name: ListPipelineEdgesByWorkspace :many
SELECT * FROM pipeline_edge
WHERE workspace_id = $1
ORDER BY created_at ASC;

-- name: ListPipelineEdgesByParentIssue :many
-- Returns all edges that connect sub-issues of a given parent issue.
-- Used to reconstruct the full pipeline DAG for visualization.
SELECT pe.* FROM pipeline_edge pe
JOIN issue src ON src.id = pe.source_issue_id
WHERE src.parent_issue_id = $1
   OR pe.source_issue_id = $1
ORDER BY pe.created_at ASC;

-- name: ListPipelineEdgesBySourceIssue :many
SELECT * FROM pipeline_edge
WHERE source_issue_id = $1
ORDER BY created_at ASC;

-- name: ListPipelineEdgesByTargetIssue :many
SELECT * FROM pipeline_edge
WHERE target_issue_id = $1
ORDER BY created_at ASC;

-- name: DeletePipelineEdge :exec
DELETE FROM pipeline_edge WHERE id = $1;

-- name: UpdatePipelineEdgeMetadata :one
UPDATE pipeline_edge
SET metadata = $2
WHERE id = $1
RETURNING *;

-- Pipeline Annotation CRUD

-- name: CreatePipelineAnnotation :one
INSERT INTO pipeline_annotation (edge_id, author_id, content, annotation_type)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListAnnotationsByEdge :many
SELECT * FROM pipeline_annotation
WHERE edge_id = $1
ORDER BY created_at ASC;

-- name: DeletePipelineAnnotation :exec
DELETE FROM pipeline_annotation WHERE id = $1;
