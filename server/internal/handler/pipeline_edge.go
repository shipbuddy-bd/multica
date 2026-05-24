package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// --- Pipeline Edge API ---

type CreatePipelineEdgeRequest struct {
	SourceIssueID string `json:"source_issue_id"`
	TargetIssueID string `json:"target_issue_id"`
	EdgeType      string `json:"edge_type"`
	Metadata      any    `json:"metadata,omitempty"`
}

func (h *Handler) CreatePipelineEdge(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}

	var req CreatePipelineEdgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sourceID, ok := parseUUIDOrBadRequest(w, req.SourceIssueID, "source_issue_id")
	if !ok {
		return
	}
	targetID, ok := parseUUIDOrBadRequest(w, req.TargetIssueID, "target_issue_id")
	if !ok {
		return
	}

	edgeType := req.EdgeType
	if edgeType == "" {
		edgeType = "flow"
	}

	var metaBytes []byte
	if req.Metadata != nil {
		metaBytes, _ = json.Marshal(req.Metadata)
	} else {
		metaBytes = []byte("{}")
	}

	edge, err := h.Queries.CreatePipelineEdge(r.Context(), db.CreatePipelineEdgeParams{
		WorkspaceID:   wsUUID,
		SourceIssueID: sourceID,
		TargetIssueID: targetID,
		EdgeType:      edgeType,
		Metadata:      metaBytes,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create pipeline edge")
		return
	}

	writeJSON(w, http.StatusCreated, edge)
}

func (h *Handler) ListPipelineEdges(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}

	parentIssueIDStr := r.URL.Query().Get("parent_issue_id")
	if parentIssueIDStr != "" {
		parentIssueID, ok := parseUUIDOrBadRequest(w, parentIssueIDStr, "parent_issue_id")
		if !ok {
			return
		}
		edges, err := h.Queries.ListPipelineEdgesByParentIssue(r.Context(), parentIssueID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list edges")
			return
		}
		writeJSON(w, http.StatusOK, edges)
		return
	}

	edges, err := h.Queries.ListPipelineEdgesByWorkspace(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list edges")
		return
	}
	writeJSON(w, http.StatusOK, edges)
}

// GetPipelineDAG returns the full pipeline DAG for a given root issue.
func (h *Handler) GetPipelineDAG(w http.ResponseWriter, r *http.Request) {
	issueIDStr := chi.URLParam(r, "issueId")
	issueID, ok := parseUUIDOrBadRequest(w, issueIDStr, "issueId")
	if !ok {
		return
	}

	// Get all edges related to this pipeline
	edges, err := h.Queries.ListPipelineEdgesByParentIssue(r.Context(), issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list edges")
		return
	}

	// Get all child issues
	childIssues, err := h.Queries.ListChildIssues(r.Context(), issueID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"edges":  edges,
			"issues": []any{},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"edges":  edges,
		"issues": childIssues,
	})
}

// --- Pipeline Annotation API ---

type CreateAnnotationRequest struct {
	Content        string `json:"content"`
	AnnotationType string `json:"annotation_type"`
}

func (h *Handler) CreatePipelineAnnotation(w http.ResponseWriter, r *http.Request) {
	edgeIDStr := chi.URLParam(r, "edgeId")
	edgeID, ok := parseUUIDOrBadRequest(w, edgeIDStr, "edgeId")
	if !ok {
		return
	}

	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	userUUID := parseUUID(userID)

	var req CreateAnnotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if req.AnnotationType == "" {
		req.AnnotationType = "note"
	}

	annotation, err := h.Queries.CreatePipelineAnnotation(r.Context(), db.CreatePipelineAnnotationParams{
		EdgeID:         edgeID,
		AuthorID:       userUUID,
		Content:        req.Content,
		AnnotationType: req.AnnotationType,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create annotation")
		return
	}

	writeJSON(w, http.StatusCreated, annotation)
}

func (h *Handler) ListPipelineAnnotations(w http.ResponseWriter, r *http.Request) {
	edgeIDStr := chi.URLParam(r, "edgeId")
	edgeID, ok := parseUUIDOrBadRequest(w, edgeIDStr, "edgeId")
	if !ok {
		return
	}

	annotations, err := h.Queries.ListAnnotationsByEdge(r.Context(), edgeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list annotations")
		return
	}

	writeJSON(w, http.StatusOK, annotations)
}
