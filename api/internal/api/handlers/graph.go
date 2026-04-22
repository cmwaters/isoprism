package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/isoprism/api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GraphHandler struct {
	DB *pgxpool.Pool
}

// GET /api/v1/repos/{repoID}/prs/{prID}/graph
func (h *GraphHandler) GetGraph(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	prID := chi.URLParam(r, "prID")
	userID := r.Header.Get("X-User-ID")
	ctx := r.Context()

	// Verify repo ownership
	var exists bool
	h.DB.QueryRow(ctx, `select exists(select 1 from repositories where id=$1 and user_id=$2)`, repoID, userID).Scan(&exists)
	if !exists {
		http.Error(w, "repo not found", http.StatusNotFound)
		return
	}

	// Load PR
	var pr models.GraphPR
	var baseCommit, headCommit string
	err := h.DB.QueryRow(ctx, `
		select id, number, title, html_url,
		       coalesce(base_commit_sha,''), coalesce(head_commit_sha,'')
		from pull_requests where id=$1 and repo_id=$2
	`, prID, repoID).Scan(&pr.ID, &pr.Number, &pr.Title, &pr.HTMLURL, &baseCommit, &headCommit)
	if err != nil {
		http.Error(w, "pr not found", http.StatusNotFound)
		return
	}
	pr.BaseCommitSHA = baseCommit
	pr.HeadCommitSHA = headCommit

	// Load changed nodes (directly modified)
	type rawChange struct {
		nodeID        string
		changeType    string
		changeSummary *string
		diffHunk      *string
	}
	changedRows, err := h.DB.Query(ctx, `
		select pnc.node_id, pnc.change_type, pnc.change_summary, pnc.diff_hunk
		from pr_node_changes pnc
		where pnc.pull_request_id = $1
	`, prID)
	if err != nil {
		log.Printf("GetGraph: pr_node_changes query: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer changedRows.Close()

	changedSet := map[string]rawChange{}
	var changedIDs []string
	for changedRows.Next() {
		var c rawChange
		if err := changedRows.Scan(&c.nodeID, &c.changeType, &c.changeSummary, &c.diffHunk); err != nil {
			continue
		}
		changedSet[c.nodeID] = c
		changedIDs = append(changedIDs, c.nodeID)
	}
	changedRows.Close()

	if len(changedIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.GraphResponse{
			PR:    pr,
			Nodes: []models.GraphNode{},
			Edges: []models.GraphEdge{},
		})
		return
	}

	// One-hop expansion: find callers and callees of changed nodes
	// Use the head commit for context
	type edgeRow struct {
		callerID string
		calleeID string
	}
	var allEdges []edgeRow
	callerSet := map[string]bool{} // nodes that call a changed node
	calleeSet := map[string]bool{} // nodes called by a changed node

	edgeRows, _ := h.DB.Query(ctx, `
		select ce.caller_id, ce.callee_id
		from code_edges ce
		join code_nodes cn on cn.id = ce.caller_id or cn.id = ce.callee_id
		where cn.repo_id = $1
		  and cn.commit_sha = $2
		  and (ce.caller_id = any($3::uuid[]) or ce.callee_id = any($3::uuid[]))
	`, repoID, headCommit, changedIDs)
	if edgeRows != nil {
		defer edgeRows.Close()
		for edgeRows.Next() {
			var e edgeRow
			if err := edgeRows.Scan(&e.callerID, &e.calleeID); err != nil {
				continue
			}
			allEdges = append(allEdges, e)
			for _, cid := range changedIDs {
				if e.calleeID == cid {
					callerSet[e.callerID] = true
				}
				if e.callerID == cid {
					calleeSet[e.calleeID] = true
				}
			}
		}
	}

	// Build the full node ID set (cap at 20)
	includedIDs := map[string]bool{}
	for _, id := range changedIDs {
		includedIDs[id] = true
	}
	for id := range callerSet {
		if !includedIDs[id] && len(includedIDs) < 20 {
			includedIDs[id] = true
		}
	}
	for id := range calleeSet {
		if !includedIDs[id] && len(includedIDs) < 20 {
			includedIDs[id] = true
		}
	}

	// Fetch node details
	idList := make([]string, 0, len(includedIDs))
	for id := range includedIDs {
		idList = append(idList, id)
	}

	nodeRows, err := h.DB.Query(ctx, `
		select id, name, full_name, file_path, line_start, line_end,
		       signature, language, kind, coalesce(summary,'')
		from code_nodes where id = any($1::uuid[])
	`, idList)
	if err != nil {
		log.Printf("GetGraph: node query: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer nodeRows.Close()

	nodeMap := map[string]models.GraphNode{}
	for nodeRows.Next() {
		var n models.GraphNode
		var summary string
		if err := nodeRows.Scan(
			&n.ID, &n.Name, &n.FullName, &n.FilePath, &n.LineStart, &n.LineEnd,
			&n.Signature, &n.Language, &n.Kind, &summary,
		); err != nil {
			continue
		}
		if summary != "" {
			n.Summary = &summary
		}

		// Tag node type
		if _, isChanged := changedSet[n.ID]; isChanged {
			n.NodeType = "changed"
			c := changedSet[n.ID]
			n.ChangeSummary = c.changeSummary
			n.DiffHunk = c.diffHunk
			ct := c.changeType
			n.ChangeType = &ct
			if c.diffHunk != nil {
				added, removed := countDiffLines(*c.diffHunk)
				n.LinesAdded = added
				n.LinesRemoved = removed
			}
		} else if callerSet[n.ID] {
			n.NodeType = "caller"
		} else {
			n.NodeType = "callee"
		}

		nodeMap[n.ID] = n
	}

	// Build response nodes and filter edges to only included nodes
	nodes := make([]models.GraphNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}

	edges := make([]models.GraphEdge, 0)
	for _, e := range allEdges {
		if includedIDs[e.callerID] && includedIDs[e.calleeID] {
			edges = append(edges, models.GraphEdge{
				CallerID: e.callerID,
				CalleeID: e.calleeID,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.GraphResponse{
		PR:    pr,
		Nodes: nodes,
		Edges: edges,
	})
}

func countDiffLines(patch string) (added, removed int) {
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return
}
