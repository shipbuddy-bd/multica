import { useEffect, useRef, useState } from "react";
import type { PipelineEdge } from "@multica/core/types";
import { parseEdgeMetadata } from "@multica/core/types";

/** A normalized edge with extracted score for rendering. */
interface RenderEdge {
  id: string;
  edge: PipelineEdge;
  fromX: number;
  fromY: number;
  toX: number;
  toY: number;
  score?: number;
}

/** Pixel coordinates for a card's anchor points. */
interface CardRect {
  left: number;
  top: number;
  right: number;
  bottom: number;
  centerX: number;
  centerY: number;
}

/**
 * BoardEdgeOverlay renders SVG arrows between issue cards on the board.
 *
 * Positioning works by:
 * 1. Finding the board container element (passed via `containerRef`)
 * 2. Looking up DOM nodes with `[data-issue-id="..."]` for each edge endpoint
 * 3. Computing card rects relative to the container
 * 4. Drawing a curved path with an arrowhead from source to target
 *
 * The overlay re-measures positions on resize, scroll, and edge data changes.
 * It uses `pointer-events: none` on the SVG so cards underneath remain
 * interactive — only the path elements opt back in via `pointer-events: auto`.
 */
interface BoardEdgeOverlayProps {
  edges: PipelineEdge[];
  containerRef: React.RefObject<HTMLDivElement | null>;
  onEdgeClick?: (edge: PipelineEdge) => void;
}

export function BoardEdgeOverlay({
  edges,
  containerRef,
  onEdgeClick,
}: BoardEdgeOverlayProps) {
  const [renderEdges, setRenderEdges] = useState<RenderEdge[]>([]);
  const [size, setSize] = useState<{ width: number; height: number }>({
    width: 0,
    height: 0,
  });
  const rafRef = useRef<number | null>(null);

  // Measure card positions and compute edge geometry.
  useEffect(() => {
    if (!containerRef.current) return;
    const container = containerRef.current;

    const measure = () => {
      const containerRect = container.getBoundingClientRect();
      setSize({ width: containerRect.width, height: containerRect.height });

      const cardRects: Record<string, CardRect> = {};
      const nodes = container.querySelectorAll<HTMLElement>("[data-issue-id]");
      nodes.forEach((el) => {
        const id = el.getAttribute("data-issue-id");
        if (!id) return;
        const r = el.getBoundingClientRect();
        cardRects[id] = {
          left: r.left - containerRect.left + container.scrollLeft,
          top: r.top - containerRect.top + container.scrollTop,
          right: r.right - containerRect.left + container.scrollLeft,
          bottom: r.bottom - containerRect.top + container.scrollTop,
          centerX:
            (r.left + r.right) / 2 - containerRect.left + container.scrollLeft,
          centerY:
            (r.top + r.bottom) / 2 - containerRect.top + container.scrollTop,
        };
      });

      const next: RenderEdge[] = [];
      for (const edge of edges) {
        const src = cardRects[edge.source_issue_id];
        const dst = cardRects[edge.target_issue_id];
        if (!src || !dst) continue; // card not rendered (filtered out / paginated away)

        // Use right side of source, left side of target (left-to-right flow).
        const fromX = src.right;
        const fromY = src.centerY;
        const toX = dst.left;
        const toY = dst.centerY;

        const meta = parseEdgeMetadata(edge.metadata);
        const score = meta.score?.total;

        next.push({ id: edge.id, edge, fromX, fromY, toX, toY, score });
      }
      setRenderEdges(next);
    };

    const scheduleMeasure = () => {
      if (rafRef.current !== null) cancelAnimationFrame(rafRef.current);
      rafRef.current = requestAnimationFrame(() => {
        rafRef.current = null;
        measure();
      });
    };

    measure();

    const ro = new ResizeObserver(scheduleMeasure);
    ro.observe(container);
    container.addEventListener("scroll", scheduleMeasure, { passive: true });
    window.addEventListener("resize", scheduleMeasure);

    // Re-measure after dnd-kit transitions and async layout updates.
    const interval = setInterval(scheduleMeasure, 500);

    return () => {
      ro.disconnect();
      container.removeEventListener("scroll", scheduleMeasure);
      window.removeEventListener("resize", scheduleMeasure);
      if (rafRef.current !== null) cancelAnimationFrame(rafRef.current);
      clearInterval(interval);
    };
  }, [containerRef, edges]);

  if (size.width === 0 || renderEdges.length === 0) return null;

  return (
    <svg
      className="absolute inset-0 pointer-events-none"
      width={size.width}
      height={size.height}
      style={{ overflow: "visible" }}
      aria-hidden
    >
      <defs>
        <marker
          id="multica-pipeline-arrow"
          viewBox="0 0 10 10"
          refX="9"
          refY="5"
          markerWidth="6"
          markerHeight="6"
          orient="auto-start-reverse"
        >
          <path d="M 0 0 L 10 5 L 0 10 z" fill="currentColor" />
        </marker>
      </defs>

      {renderEdges.map((re) => {
        const { fromX, fromY, toX, toY, edge, score } = re;
        // Cubic bezier with horizontal control points for a smooth left-right flow.
        const dx = Math.max(40, (toX - fromX) / 2);
        const path = `M ${fromX} ${fromY} C ${fromX + dx} ${fromY}, ${toX - dx} ${toY}, ${toX} ${toY}`;
        const midX = (fromX + toX) / 2;
        const midY = (fromY + toY) / 2;

        const colorClass = edgeColorClass(edge.edge_type);
        return (
          <g
            key={re.id}
            className={`pointer-events-auto cursor-pointer ${colorClass}`}
            onClick={() => onEdgeClick?.(edge)}
          >
            {/* Wide invisible hit area for easier clicking */}
            <path
              d={path}
              stroke="transparent"
              strokeWidth={14}
              fill="none"
            />
            {/* Visible path */}
            <path
              d={path}
              stroke="currentColor"
              strokeWidth={2}
              fill="none"
              markerEnd="url(#multica-pipeline-arrow)"
              className="transition-[stroke-width] hover:stroke-[3]"
            />
            {/* Score badge at midpoint */}
            {typeof score === "number" && (
              <g transform={`translate(${midX} ${midY})`}>
                <rect
                  x={-18}
                  y={-10}
                  width={36}
                  height={20}
                  rx={10}
                  fill="white"
                  stroke="currentColor"
                  strokeWidth={1}
                  className="dark:fill-background"
                />
                <text
                  x={0}
                  y={4}
                  fontSize={11}
                  textAnchor="middle"
                  fill="currentColor"
                  fontWeight={600}
                >
                  {Math.round(score)}
                </text>
              </g>
            )}
          </g>
        );
      })}
    </svg>
  );
}

/** Tailwind color classes per edge type (sets `currentColor` for the SVG). */
function edgeColorClass(edgeType: string): string {
  switch (edgeType) {
    case "branch":
      return "text-blue-500";
    case "merge":
      return "text-purple-500";
    case "retry":
      return "text-amber-500";
    case "flow":
    default:
      return "text-emerald-500";
  }
}
