package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Yugp72/recurseai/core"
	_ "modernc.org/sqlite"
)

type Tree = core.Tree
type TreeNode = core.TreeNode

type TreeStore interface {
	SaveTree(ctx context.Context, tree *Tree) error
	LoadTree(ctx context.Context, docID string) (*Tree, error)
	SaveNode(ctx context.Context, node *TreeNode) error
	LoadNode(ctx context.Context, nodeID string) (*TreeNode, error)
	ListTrees(ctx context.Context) ([]string, error)
	DeleteTree(ctx context.Context, docID string) error
}

type SQLiteTreeStore struct {
	db   *sql.DB
	path string
}

func NewSQLiteTreeStore(path string) (*SQLiteTreeStore, error) {
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

	s := &SQLiteTreeStore{db: db, path: path}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

func (s *SQLiteTreeStore) SaveTree(ctx context.Context, tree *Tree) error {
	if s == nil || s.db == nil {
		return errors.New("tree store is not initialized")
	}
	if tree == nil {
		return errors.New("tree is required")
	}
	if tree.DocID == "" {
		return errors.New("tree docID is required")
	}

	treeBlob, err := json.Marshal(tree)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO trees (doc_id, tree_data)
		VALUES (?, ?)
		ON CONFLICT(doc_id) DO UPDATE SET
			tree_data=excluded.tree_data,
			updated_at=CURRENT_TIMESTAMP
	`, tree.DocID, treeBlob); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM tree_node_refs WHERE doc_id = ?`, tree.DocID); err != nil {
		return err
	}

	nodes := flattenNodes(tree.Root)
	for _, node := range nodes {
		nodeBlob, err := s.serializeNode(node)
		if err != nil {
			return err
		}

		if _, err = tx.ExecContext(ctx, `
			INSERT INTO nodes (node_id, node_data)
			VALUES (?, ?)
			ON CONFLICT(node_id) DO UPDATE SET
				node_data=excluded.node_data,
				updated_at=CURRENT_TIMESTAMP
		`, node.ID, nodeBlob); err != nil {
			return err
		}

		if _, err = tx.ExecContext(ctx, `
			INSERT INTO tree_node_refs (doc_id, node_id)
			VALUES (?, ?)
			ON CONFLICT(doc_id, node_id) DO NOTHING
		`, tree.DocID, node.ID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteTreeStore) LoadTree(ctx context.Context, docID string) (*Tree, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tree store is not initialized")
	}
	if docID == "" {
		return nil, errors.New("docID is required")
	}

	var blob []byte
	err := s.db.QueryRowContext(ctx, `SELECT tree_data FROM trees WHERE doc_id = ?`, docID).Scan(&blob)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("tree not found: %s", docID)
		}
		return nil, err
	}

	var t Tree
	if err := json.Unmarshal(blob, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *SQLiteTreeStore) SaveNode(ctx context.Context, node *TreeNode) error {
	if s == nil || s.db == nil {
		return errors.New("tree store is not initialized")
	}
	if node == nil {
		return errors.New("node is required")
	}
	if node.ID == "" {
		return errors.New("node ID is required")
	}

	blob, err := s.serializeNode(node)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO nodes (node_id, node_data)
		VALUES (?, ?)
		ON CONFLICT(node_id) DO UPDATE SET
			node_data=excluded.node_data,
			updated_at=CURRENT_TIMESTAMP
	`, node.ID, blob)
	return err
}

func (s *SQLiteTreeStore) LoadNode(ctx context.Context, nodeID string) (*TreeNode, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tree store is not initialized")
	}
	if nodeID == "" {
		return nil, errors.New("nodeID is required")
	}

	var blob []byte
	err := s.db.QueryRowContext(ctx, `SELECT node_data FROM nodes WHERE node_id = ?`, nodeID).Scan(&blob)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("node not found: %s", nodeID)
		}
		return nil, err
	}

	return s.deserializeNode(blob)
}

func (s *SQLiteTreeStore) ListTrees(ctx context.Context) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tree store is not initialized")
	}

	rows, err := s.db.QueryContext(ctx, `SELECT doc_id FROM trees`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Strings(ids)
	return ids, nil
}

func (s *SQLiteTreeStore) DeleteTree(ctx context.Context, docID string) error {
	if s == nil || s.db == nil {
		return errors.New("tree store is not initialized")
	}
	if docID == "" {
		return errors.New("docID is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.ExecContext(ctx, `DELETE FROM trees WHERE doc_id = ?`, docID); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
		DELETE FROM nodes
		WHERE node_id IN (SELECT node_id FROM tree_node_refs WHERE doc_id = ?)
	`, docID); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM tree_node_refs WHERE doc_id = ?`, docID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SQLiteTreeStore) initSchema() error {
	if s == nil || s.db == nil {
		return errors.New("tree store is not initialized")
	}

	stmt := `
	CREATE TABLE IF NOT EXISTS trees (
		doc_id TEXT PRIMARY KEY,
		tree_data BLOB NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS nodes (
		node_id TEXT PRIMARY KEY,
		node_data BLOB NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tree_node_refs (
		doc_id TEXT NOT NULL,
		node_id TEXT NOT NULL,
		PRIMARY KEY (doc_id, node_id)
	);

	CREATE INDEX IF NOT EXISTS idx_tree_node_refs_doc ON tree_node_refs(doc_id);
	`

	_, err := s.db.Exec(stmt)
	return err
}

func (s *SQLiteTreeStore) serializeNode(node *TreeNode) ([]byte, error) {
	if node == nil {
		return nil, errors.New("node is required")
	}
	return json.Marshal(node)
}

func (s *SQLiteTreeStore) deserializeNode(blob []byte) (*TreeNode, error) {
	if len(blob) == 0 {
		return nil, errors.New("empty node payload")
	}
	var node TreeNode
	if err := json.Unmarshal(blob, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

func (s *SQLiteTreeStore) GetLatestTree(ctx context.Context) (*Tree, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tree store is not initialized")
	}

	var blob []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT tree_data
		FROM trees
		ORDER BY updated_at DESC, created_at DESC
		LIMIT 1
	`).Scan(&blob)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("no trees found")
		}
		return nil, err
	}

	var t Tree
	if err := json.Unmarshal(blob, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *SQLiteTreeStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func flattenNodes(root *TreeNode) []*TreeNode {
	if root == nil {
		return nil
	}

	seen := make(map[string]bool)
	out := make([]*TreeNode, 0)
	stack := []*TreeNode{root}

	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil || seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		out = append(out, n)
		for i := len(n.Children) - 1; i >= 0; i-- {
			stack = append(stack, n.Children[i])
		}
	}

	return out
}
