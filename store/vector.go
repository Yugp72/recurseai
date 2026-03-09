package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

type VectorStore interface {
	Save(ctx context.Context, id string, embedding []float32, metadata map[string]string) error
	Search(ctx context.Context, embedding []float32, topK int) ([]SearchResult, error)
	Get(ctx context.Context, id string) ([]float32, error)
	Delete(ctx context.Context, id string) error
	Close() error
}

type SearchResult struct {
	ID       string
	Score    float64
	Metadata map[string]string
}

type SQLiteVectorStore struct {
	db   *sql.DB
	path string
}

func NewSQLiteVectorStore(path string) (*SQLiteVectorStore, error) {
	if path == "" {
		path = "recurseai.db"
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	store := &SQLiteVectorStore{db: db, path: path}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteVectorStore) Save(ctx context.Context, id string, embedding []float32, metadata map[string]string) error {
	if s == nil || s.db == nil {
		return errors.New("vector store is not initialized")
	}
	if id == "" {
		return errors.New("id is required")
	}
	if len(embedding) == 0 {
		return errors.New("embedding is required")
	}
	if metadata == nil {
		metadata = map[string]string{}
	}

	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO vectors (id, embedding, metadata)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			embedding=excluded.embedding,
			metadata=excluded.metadata
	`

	_, err = s.db.ExecContext(ctx, query, id, s.serializeVec(embedding), string(metaBytes))
	return err
}

func (s *SQLiteVectorStore) Search(ctx context.Context, embedding []float32, topK int) ([]SearchResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("vector store is not initialized")
	}
	if len(embedding) == 0 {
		return nil, errors.New("embedding is required")
	}
	if topK <= 0 {
		topK = 5
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, embedding, metadata FROM vectors`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]SearchResult, 0)
	for rows.Next() {
		var id string
		var embBlob []byte
		var metaRaw string
		if err := rows.Scan(&id, &embBlob, &metaRaw); err != nil {
			return nil, err
		}

		vec := s.deserializeVec(embBlob)
		score := cosineSimilarity(embedding, vec)

		meta := map[string]string{}
		if metaRaw != "" {
			_ = json.Unmarshal([]byte(metaRaw), &meta)
		}

		results = append(results, SearchResult{
			ID:       id,
			Score:    score,
			Metadata: meta,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

func (s *SQLiteVectorStore) Get(ctx context.Context, id string) ([]float32, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("vector store is not initialized")
	}
	if id == "" {
		return nil, errors.New("id is required")
	}

	var embBlob []byte
	err := s.db.QueryRowContext(ctx, `SELECT embedding FROM vectors WHERE id = ?`, id).Scan(&embBlob)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("vector not found: %s", id)
		}
		return nil, err
	}

	return s.deserializeVec(embBlob), nil
}

func (s *SQLiteVectorStore) Delete(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("vector store is not initialized")
	}
	if id == "" {
		return errors.New("id is required")
	}

	_, err := s.db.ExecContext(ctx, `DELETE FROM vectors WHERE id = ?`, id)
	return err
}

func (s *SQLiteVectorStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteVectorStore) initSchema() error {
	if s == nil || s.db == nil {
		return errors.New("vector store is not initialized")
	}

	stmt := `
	CREATE TABLE IF NOT EXISTS vectors (
		id TEXT PRIMARY KEY,
		embedding BLOB NOT NULL,
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_vectors_created_at ON vectors(created_at);
	`

	_, err := s.db.Exec(stmt)
	return err
}

func (s *SQLiteVectorStore) serializeVec(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	b := make([]byte, len(v)*4)
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(x))
	}
	return b
}

func (s *SQLiteVectorStore) deserializeVec(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	n := len(b) / 4
	v := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		v[i] = math.Float32frombits(bits)
	}
	return v
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	var dot float64
	var normA float64
	var normB float64
	for i := 0; i < minLen; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
