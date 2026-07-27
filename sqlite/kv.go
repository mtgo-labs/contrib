package sqlite

import (
	"database/sql"
	"fmt"
)

// KVRepo is a key-value store backed by the key_value SQLite table.
type KVRepo struct {
	drv *driver

	getStmt    *sql.Stmt
	setStmt    *sql.Stmt
	delStmt    *sql.Stmt
	delAllStmt *sql.Stmt
}

func newKVRepo(drv *driver) *KVRepo {
	r := &KVRepo{drv: drv}
	drv.registerOnLoad(r.prepare)
	return r
}

func (r *KVRepo) prepare() error {
	db := r.drv.db
	var err error

	r.getStmt, err = db.Prepare(`SELECT value FROM key_value WHERE key = ?`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare kv.get: %w", err)
	}
	r.setStmt, err = db.Prepare(`INSERT OR REPLACE INTO key_value (key, value) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare kv.set: %w", err)
	}
	r.delStmt, err = db.Prepare(`DELETE FROM key_value WHERE key = ?`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare kv.del: %w", err)
	}
	r.delAllStmt, err = db.Prepare(`DELETE FROM key_value`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare kv.delAll: %w", err)
	}

	return nil
}

// Set stores a value under the given key.
func (r *KVRepo) Set(key string, value []byte) {
	r.setStmt.Exec(key, value)
}

// Get returns the value for the given key, or nil if not found.
func (r *KVRepo) Get(key string) ([]byte, error) {
	var value []byte
	err := r.getStmt.QueryRow(key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: kv.get %q: %w", key, err)
	}
	return value, nil
}

// Delete removes the entry for the given key.
func (r *KVRepo) Delete(key string) error {
	_, err := r.delStmt.Exec(key)
	return err
}

// DeleteAll removes all key-value entries.
func (r *KVRepo) DeleteAll() error {
	_, err := r.delAllStmt.Exec()
	return err
}
