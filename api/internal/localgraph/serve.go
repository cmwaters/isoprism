package localgraph

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/isoprism/api/internal/models"
)

//go:embed viewer/*
var embeddedViewer embed.FS

// Serve starts the local HTTP daemon and opens the viewer when requested.
func Serve(ctx context.Context, opts ServeOptions) error {
	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := opts.Port
	if port == 0 {
		port = 3717
	}
	webPort := opts.WebPort
	if webPort == 0 {
		webPort = 3000
	}
	if host == "0.0.0.0" || host == "::" {
		log.Printf("warning: serving on non-loopback host %s because it was explicitly requested", host)
	}
	root, err := repoRoot(ctx, opts.RepoDir)
	if err != nil {
		return err
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(root, ".isoprism")
	}
	apiBase := "http://" + host + ":" + strconv.Itoa(port)
	webURL := ""
	if !opts.NoWeb {
		webURL = apiBase
	}

	if opts.NoWeb {
		mux := localMux(root, cacheDir, "", false)
		server := &http.Server{Addr: host + ":" + strconv.Itoa(port), Handler: mux}
		log.Printf("isoprism API listening on %s", apiBase)
		return server.ListenAndServe()
	}

	if opts.WebDir == "" && os.Getenv("ISOPRISM_WEB_DIR") == "" {
		mux := localMux(root, cacheDir, webURL, true)
		server := &http.Server{Addr: host + ":" + strconv.Itoa(port), Handler: mux}
		log.Printf("isoprism API listening on %s", apiBase)
		log.Printf("isoprism web listening on %s", webURL)
		log.Printf("open %s", webURL)
		return server.ListenAndServe()
	}

	webDir, err := resolveWebDir(opts.WebDir)
	if err != nil {
		return err
	}
	webURL = "http://127.0.0.1:" + strconv.Itoa(webPort) + "/local"
	mux := localMux(root, cacheDir, webURL, false)
	server := &http.Server{Addr: host + ":" + strconv.Itoa(port), Handler: mux}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("isoprism API listening on %s", apiBase)
		errCh <- server.ListenAndServe()
	}()

	cmd := exec.CommandContext(ctx, "npm", "run", "dev", "--", "--webpack", "--hostname", "127.0.0.1", "--port", strconv.Itoa(webPort))
	cmd.Dir = webDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"NEXT_PUBLIC_API_URL="+apiBase,
		"NEXT_PUBLIC_ISOPRISM_LOCAL_API_URL="+apiBase,
	)
	log.Printf("isoprism web listening on %s", webURL)
	if err := cmd.Start(); err != nil {
		_ = server.Shutdown(context.Background())
		return fmt.Errorf("start local web viewer: %w", err)
	}
	go func() {
		errCh <- cmd.Wait()
	}()
	log.Printf("open %s", webURL)
	return <-errCh
}

// resolveWebDir resolves web dir for the local CLI graph runtime.
func resolveWebDir(explicit string) (string, error) {
	var candidates []string
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if env := os.Getenv("ISOPRISM_WEB_DIR"); env != "" {
		candidates = append(candidates, env)
	}
	for _, candidate := range candidates {
		webDir, ok := validWebDir(candidate)
		if ok {
			return webDir, nil
		}
	}
	return "", fmt.Errorf("could not find Isoprism web app directory; pass --web-dir <path> or set ISOPRISM_WEB_DIR")
}

// validWebDir reports whether a candidate directory contains the local web viewer.
func validWebDir(candidate string) (string, bool) {
	if candidate == "" {
		return "", false
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(filepath.Join(abs, "package.json"))
	if err != nil || info.IsDir() {
		return "", false
	}
	return abs, true
}

// localMux builds the HTTP mux for the local graph daemon.
func localMux(root, cacheDir, webURL string, serveEmbeddedViewer bool) http.Handler {
	mux := http.NewServeMux()
	opts := Options{RepoDir: root, CacheDir: cacheDir}
	var reviewPayloadMu sync.RWMutex
	reviewPayloads := map[string]ReviewGraphPayload{}

	if serveEmbeddedViewer {
		handleEmbeddedViewer(mux)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		if r.Method == http.MethodOptions {
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if serveEmbeddedViewer {
			embeddedIndex(w, r)
			return
		}
		if webURL != "" {
			http.Redirect(w, r, webURL, http.StatusFound)
			return
		}
		writeJSON(w, map[string]string{
			"status": "ok",
			"viewer": "disabled",
			"diff":   "/api/diff",
		}, nil)
	})
	mux.HandleFunc("/api/diff", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		payload, err := GenerateDiff(r.Context(), opts)
		writeJSON(w, payload, err)
	})
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		repo, err := LoadRepoMetadata(r.Context(), opts)
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		writeJSON(w, map[string]any{
			"id":   "local",
			"mode": "local",
			"repo": repo,
			"capabilities": map[string]any{
				"canCompareRefs":     true,
				"canReadWorkingTree": true,
				"canReadGitIndex":    true,
				"canUseGh":           ghAvailable(),
			},
		}, nil)
	})
	mux.HandleFunc("/api/review/compare", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			BaseRef string `json:"base_ref"`
			HeadRef string `json:"head_ref"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid review comparison payload", http.StatusBadRequest)
			return
		}
		baseRef := strings.TrimSpace(req.BaseRef)
		headRef := strings.TrimSpace(req.HeadRef)
		if baseRef == "" {
			g := gitClient{root: root}
			branch, err := g.resolveDefaultBranch(r.Context())
			if err != nil {
				writeJSON(w, nil, err)
				return
			}
			baseRef = branch
		}
		if headRef == "" {
			headRef = "worktree"
		}
		payload, err := GenerateDiff(r.Context(), Options{RepoDir: root, CacheDir: cacheDir, Args: []string{baseRef, headRef}})
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		reviewPayloadMu.Lock()
		reviewPayloads[payload.Graph.PR.ID] = payload
		reviewPayloadMu.Unlock()
		writeJSON(w, payload.Graph, nil)
	})
	handleRepo := func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		repo, err := LoadRepoMetadata(r.Context(), opts)
		writeJSON(w, repo, err)
	}
	mux.HandleFunc("/api/repo", handleRepo)
	mux.HandleFunc("/api/review-items", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		items, err := listReviewItems(r.Context(), opts)
		if err != nil {
			items = []models.QueuePR{}
		}
		writeJSON(w, map[string]any{"review_items": items}, nil)
	})
	mux.HandleFunc("/api/review-items/", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		id := strings.TrimPrefix(r.URL.Path, "/api/review-items/")
		id = strings.TrimSuffix(id, "/graph")
		reviewPayloadMu.RLock()
		payload, exists := reviewPayloads[id]
		reviewPayloadMu.RUnlock()
		if !exists {
			var err error
			if isLocalReviewItem(id) {
				payload, err = loadLocalReviewItemGraph(r.Context(), opts, id)
			} else {
				payload, err = loadGHPullRequestGraph(r.Context(), opts, id)
			}
			if err != nil {
				writeJSON(w, nil, err)
				return
			}
			reviewPayloadMu.Lock()
			reviewPayloads[id] = payload
			reviewPayloadMu.Unlock()
		}
		writeJSON(w, payload.Graph, nil)
	})
	mux.HandleFunc("/api/programs", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		data, err := LoadCommitGraph(r.Context(), opts)
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		writeJSON(w, models.RepoProgramsResponse{Repo: data.Repo, Programs: data.Programs}, nil)
	})
	mux.HandleFunc("/api/programs/", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		id := strings.TrimPrefix(r.URL.Path, "/api/programs/")
		id = strings.TrimSuffix(id, "/graph")
		graph, _, err := ProgramGraph(r.Context(), opts, id)
		writeJSON(w, graph, err)
	})
	mux.HandleFunc("/api/graph/expand", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		if r.Method == http.MethodOptions {
			return
		}
		var req models.GraphExpansionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		response, _, err := ExpandGraph(r.Context(), opts, req.NodeID, req.VisibleNodeIDs)
		writeJSON(w, response, err)
	})
	mux.HandleFunc("/api/nodes/", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		id := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
		id = strings.TrimSuffix(id, "/code")
		reviewItemID := strings.TrimSpace(r.URL.Query().Get("review_item_id"))
		if reviewItemID != "" {
			reviewPayloadMu.RLock()
			payload, exists := reviewPayloads[reviewItemID]
			reviewPayloadMu.RUnlock()
			if !exists {
				http.NotFound(w, r)
				return
			}
			code, ok := localReviewNodeCode(payload, id)
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, code, nil)
			return
		}
		data, err := LoadCommitGraph(r.Context(), opts)
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		if code, ok := data.Sources[id]; ok {
			writeJSON(w, code, nil)
			return
		}
		http.NotFound(w, r)
	})
	return mux
}

// handleEmbeddedViewer handles embedded viewer for the local CLI graph runtime.
func handleEmbeddedViewer(mux *http.ServeMux) {
	viewerFS, err := fs.Sub(embeddedViewer, "viewer")
	if err != nil {
		panic(err)
	}
	assets := http.FileServer(http.FS(viewerFS))
	mux.Handle("/assets/", assets)
}

// embeddedIndex serves the embedded local viewer HTML.
func embeddedIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		withCORS(w)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := embeddedViewer.ReadFile("viewer/index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("read embedded viewer: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// withCORS adds permissive local CORS headers.
func withCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

// localReviewNodeCode loads code for a node inside a local review graph.
func localReviewNodeCode(payload ReviewGraphPayload, nodeID string) (models.NodeCodeResponse, bool) {
	node, ok := findReviewNode(payload.Graph, nodeID)
	if !ok {
		return models.NodeCodeResponse{}, false
	}
	sources, _ := payload.Metadata["sources"].(map[string]string)
	source := sources[nodeID]
	patch := node.DiffHunk
	if source == "" && patch == nil {
		return models.NodeCodeResponse{}, false
	}

	response := models.NodeCodeResponse{
		NodeID:     node.ID,
		FilePath:   node.FilePath,
		Language:   node.Language,
		DiffHunk:   patch,
		ChangeType: node.ChangeType,
	}
	if source != "" {
		segment := &models.NodeCodeSegment{
			CommitSHA: payload.Graph.PR.HeadCommitSHA,
			StartLine: node.LineStart,
			EndLine:   node.LineEnd,
			Source:    source,
		}
		if node.ChangeType != nil && *node.ChangeType == "deleted" {
			segment.CommitSHA = payload.Graph.PR.BaseCommitSHA
			response.Base = segment
		} else {
			response.Head = segment
		}
	}
	return response, true
}

// findReviewNode finds review node for the local CLI graph runtime.
func findReviewNode(graph models.GraphResponse, nodeID string) (models.GraphNode, bool) {
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	for _, node := range graph.TestChanges {
		if node.ID == nodeID {
			return node, true
		}
	}
	for _, node := range graph.TestContext {
		if node.ID == nodeID {
			return node, true
		}
	}
	return models.GraphNode{}, false
}

// writeJSON writes JSON for the local CLI graph runtime.
func writeJSON(w http.ResponseWriter, value any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		http.Error(w, fmt.Sprintf("json encode: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
