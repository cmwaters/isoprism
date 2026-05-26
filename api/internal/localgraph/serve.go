package localgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/isoprism/api/internal/models"
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
		webURL = "http://127.0.0.1:" + strconv.Itoa(webPort) + "/local"
	}
	mux := localMux(root, cacheDir, webURL)
	server := &http.Server{Addr: host + ":" + strconv.Itoa(port), Handler: mux}

	if opts.NoWeb {
		log.Printf("isoprism API listening on %s", apiBase)
		return server.ListenAndServe()
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("isoprism API listening on %s", apiBase)
		errCh <- server.ListenAndServe()
	}()

	cmd := exec.CommandContext(ctx, "npm", "run", "dev", "--", "--hostname", "127.0.0.1", "--port", strconv.Itoa(webPort))
	cmd.Dir = filepath.Join(root, "web")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "NEXT_PUBLIC_API_URL="+apiBase)
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

func localMux(root, cacheDir, webURL string) http.Handler {
	mux := http.NewServeMux()
	opts := Options{RepoDir: root, CacheDir: cacheDir}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		if r.Method == http.MethodOptions {
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
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
	mux.HandleFunc("/api/v1/local/repo", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		data, err := GenerateRepo(r.Context(), opts)
		writeJSON(w, data.Repo, err)
	})
	mux.HandleFunc("/api/v1/repos/local/queue", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		writeJSON(w, map[string]any{"prs": []models.QueuePR{}}, nil)
	})
	mux.HandleFunc("/api/v1/repos/local/programs", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		data, err := GenerateRepo(r.Context(), opts)
		if err != nil {
			writeJSON(w, nil, err)
			return
		}
		writeJSON(w, models.RepoProgramsResponse{Repo: data.Repo, Programs: data.Programs}, nil)
	})
	mux.HandleFunc("/api/v1/repos/local/programs/", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/local/programs/")
		id = strings.TrimSuffix(id, "/graph")
		graph, _, err := ProgramGraph(r.Context(), opts, id)
		writeJSON(w, graph, err)
	})
	mux.HandleFunc("/api/v1/repos/local/graph/expand", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/api/v1/repos/local/nodes/", func(w http.ResponseWriter, r *http.Request) {
		withCORS(w)
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/local/nodes/")
		id = strings.TrimSuffix(id, "/code")
		data, err := GenerateRepo(r.Context(), opts)
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

func withCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
