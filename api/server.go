package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Yugp72/recurseai/core"
)

type Engine = core.Engine

type Server struct {
	engine *Engine
	router *http.ServeMux
	port   int
}

type IngestRequest struct {
	FilePath string `json:"file_path"`
	DocID    string `json:"doc_id"`
}

type IngestResponse struct {
	DocID      string `json:"doc_id"`
	ChunkCount int    `json:"chunk_count"`
	TimeTaken  string `json:"time_taken"`
}

type QueryRequest struct {
	Question string `json:"question"`
	DocID    string `json:"doc_id"`
	Provider string `json:"provider"`
}

type QueryResponse struct {
	Answer   string   `json:"answer"`
	Sources  []string `json:"sources"`
	Provider string   `json:"provider"`
}

func NewServer(engine *Engine, port int) *Server {
	if port <= 0 {
		port = 8080
	}

	s := &Server{
		engine: engine,
		router: http.NewServeMux(),
		port:   port,
	}

	s.router.HandleFunc("/ingest", s.handleIngest)
	s.router.HandleFunc("/query", s.handleQuery)
	s.router.HandleFunc("/health", s.handleHealth)
	s.router.HandleFunc("/docs", s.handleListDocs)

	return s
}

func (s *Server) Start() error {
	if s == nil || s.router == nil {
		return errors.New("server is not initialized")
	}
	addr := ":" + strconv.Itoa(s.port)
	return http.ListenAndServe(addr, s.router)
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.engine == nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "engine not configured"})
		return
	}

	var req IngestRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.FilePath) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file_path is required"})
		return
	}

	res, err := s.engine.Ingest(r.Context(), req.FilePath)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := IngestResponse{
		DocID:      res.DocID,
		ChunkCount: res.ChunkCount,
		TimeTaken:  res.TimeTaken.String(),
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.engine == nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "engine not configured"})
		return
	}

	var req QueryRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Question) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "question is required"})
		return
	}

	var (
		res core.QueryResult
		err error
	)
	if strings.TrimSpace(req.Provider) != "" {
		res, err = s.engine.QueryWithProvider(r.Context(), req.Question, strings.TrimSpace(req.Provider))
	} else {
		res, err = s.engine.Query(r.Context(), req.Question)
	}
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	sources := make([]string, 0, len(res.Sources))
	for _, c := range res.Sources {
		src := c.SourceFile
		if src == "" {
			src = c.ID
		}
		sources = append(sources, src)
	}

	resp := QueryResponse{
		Answer:   res.Answer,
		Sources:  sources,
		Provider: res.Provider,
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.engine == nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "engine not configured"})
		return
	}

	docs, err := s.engine.ListDocs(r.Context())
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"docs": docs})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}
