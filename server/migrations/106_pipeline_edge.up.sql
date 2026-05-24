-- Pipeline Edge: tracks flow between pipeline checkpoints (issue → issue)
-- Each checkpoint in a pipeline is a sub-issue; edges record transitions and metadata.

CREATE TABLE pipeline_edge (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    source_issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    target_issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    edge_type TEXT NOT NULL DEFAULT 'flow'
        CHECK (edge_type IN ('flow', 'branch', 'merge', 'retry')),
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pipeline_edge_source ON pipeline_edge(source_issue_id);
CREATE INDEX idx_pipeline_edge_target ON pipeline_edge(target_issue_id);
CREATE INDEX idx_pipeline_edge_workspace ON pipeline_edge(workspace_id);

-- Pipeline Annotation: user notes attached to an edge (human-in-the-loop feedback)
CREATE TABLE pipeline_annotation (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    edge_id UUID NOT NULL REFERENCES pipeline_edge(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES "user"(id),
    content TEXT NOT NULL,
    annotation_type TEXT NOT NULL DEFAULT 'note'
        CHECK (annotation_type IN ('note', 'approval', 'rejection', 'suggestion')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pipeline_annotation_edge ON pipeline_annotation(edge_id);
