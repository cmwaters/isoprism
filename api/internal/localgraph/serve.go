package localgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

func Serve(ctx context.Context, opts ServeOptions) error {
	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := opts.Port
	if port == 0 {
		port = 3717
	}
	if host == "0.0.0.0" || host == "::" {
		log.Printf("warning: serving on non-loopback host %s because it was explicitly requested", host)
	}
	root, err := repoRoot(ctx, opts.RepoDir)
	if err != nil {
		return err
	}
	g := gitClient{root: root}
	defaultBranch, err := g.resolveDefaultBranch(ctx)
	if err != nil {
		return err
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir = root + "/.isoprism"
	}
	load := func() (ReviewGraphPayload, error) {
		return GenerateDiff(ctx, Options{RepoDir: root, Args: []string{defaultBranch, "HEAD"}, CacheDir: cacheDir})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		payload, err := load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		html, err := RenderStaticHTML(payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(html)
	})
	mux.HandleFunc("/api/diff", func(w http.ResponseWriter, r *http.Request) {
		payload, err := load()
		writeJSON(w, payload, err)
	})
	mux.HandleFunc("/api/repo/programs", func(w http.ResponseWriter, r *http.Request) {
		payload, err := load()
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		programs := []modelsGraphProgram{}
		for _, node := range append(payload.Graph.Nodes, payload.Graph.TestContext...) {
			if node.IsEntrypoint || node.NodeType == "changed" {
				programs = append(programs, modelsGraphProgram{ID: node.ID, FullName: node.FullName, FilePath: node.FilePath, LineStart: node.LineStart, LineEnd: node.LineEnd, Language: node.Language, Kind: node.Kind, IsEntrypoint: node.IsEntrypoint})
			}
		}
		writeJSON(w, map[string]any{"repo": payload.Repository, "programs": programs}, nil)
	})
	mux.HandleFunc("/api/repo/programs/", func(w http.ResponseWriter, r *http.Request) {
		payload, err := load()
		writeJSON(w, payload.Graph, err)
	})
	mux.HandleFunc("/api/repo/graph/expand", func(w http.ResponseWriter, r *http.Request) {
		payload, err := load()
		writeJSON(w, map[string]any{"nodes": payload.Graph.Nodes, "edges": payload.Graph.Edges, "expanded_node_id": "", "has_more": false, "hidden_neighbor_count": 0}, err)
	})
	mux.HandleFunc("/api/nodes/", func(w http.ResponseWriter, r *http.Request) {
		payload, err := load()
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		id := r.URL.Path[len("/api/nodes/"):]
		if len(id) > len("/code") && id[len(id)-len("/code"):] == "/code" {
			id = id[:len(id)-len("/code")]
		}
		for _, node := range append(payload.Graph.Nodes, payload.Graph.TestChanges...) {
			if node.ID == id {
				source := ""
				if sources, ok := payload.Metadata["sources"].(map[string]string); ok {
					source = sources[id]
				}
				writeJSON(w, map[string]any{"node_id": id, "file_path": node.FilePath, "line_start": node.LineStart, "line_end": node.LineEnd, "source": source}, nil)
				return
			}
		}
		http.NotFound(w, r)
	})
	addr := host + ":" + strconv.Itoa(port)
	log.Printf("isoprism serve listening on http://%s", addr)
	return http.ListenAndServe(addr, mux)
}

type modelsGraphProgram struct {
	ID           string `json:"id"`
	FullName     string `json:"full_name"`
	FilePath     string `json:"file_path"`
	LineStart    int    `json:"line_start"`
	LineEnd      int    `json:"line_end"`
	Language     string `json:"language"`
	Kind         string `json:"kind"`
	IsEntrypoint bool   `json:"is_entrypoint"`
}

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
