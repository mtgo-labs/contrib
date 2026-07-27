package sqlite

import (
	"database/sql"
	"fmt"
)

// AuthKeysRepo stores permanent and temporary authorization keys per DC.
type AuthKeysRepo struct {
	drv *driver

	getStmt    *sql.Stmt
	setStmt    *sql.Stmt
	delStmt    *sql.Stmt
	delAllStmt *sql.Stmt

	getTempStmt    *sql.Stmt
	setTempStmt    *sql.Stmt
	delTempStmt    *sql.Stmt
	delTempAllStmt *sql.Stmt
}

func newAuthKeysRepo(drv *driver) *AuthKeysRepo {
	r := &AuthKeysRepo{drv: drv}
	drv.registerOnLoad(r.prepare)
	return r
}

func (r *AuthKeysRepo) prepare() error {
	db := r.drv.db
	var err error

	r.getStmt, err = db.Prepare(`SELECT key FROM auth_keys WHERE dc = ?`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare auth_keys.get: %w", err)
	}
	r.setStmt, err = db.Prepare(`INSERT OR REPLACE INTO auth_keys (dc, key) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare auth_keys.set: %w", err)
	}
	r.delStmt, err = db.Prepare(`DELETE FROM auth_keys WHERE dc = ?`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare auth_keys.del: %w", err)
	}
	r.delAllStmt, err = db.Prepare(`DELETE FROM auth_keys`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare auth_keys.delAll: %w", err)
	}

	r.getTempStmt, err = db.Prepare(
		`SELECT key FROM temp_auth_keys WHERE dc = ? AND idx = ? AND expires > ?`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare temp_auth_keys.get: %w", err)
	}
	r.setTempStmt, err = db.Prepare(
		`INSERT OR REPLACE INTO temp_auth_keys (dc, idx, key, expires) VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare temp_auth_keys.set: %w", err)
	}
	r.delTempStmt, err = db.Prepare(`DELETE FROM temp_auth_keys WHERE dc = ? AND idx = ?`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare temp_auth_keys.del: %w", err)
	}
	r.delTempAllStmt, err = db.Prepare(`DELETE FROM temp_auth_keys WHERE dc = ?`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare temp_auth_keys.delAll: %w", err)
	}

	return nil
}

// Set stores a permanent auth key for the given DC. If key is nil, the
// entry is deleted instead.
func (r *AuthKeysRepo) Set(dc int, key []byte) error {
	if key == nil {
		_, err := r.delStmt.Exec(dc)
		return err
	}
	_, err := r.setStmt.Exec(dc, key)
	return err
}

// Get returns the permanent auth key for the given DC, or nil.
func (r *AuthKeysRepo) Get(dc int) ([]byte, error) {
	var key []byte
	err := r.getStmt.QueryRow(dc).Scan(&key)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: auth_keys.get dc=%d: %w", dc, err)
	}
	return key, nil
}

// SetTemp stores a temporary auth key for the given DC and index, expiring
// at the given Unix timestamp. If key is nil, the entry is deleted.
func (r *AuthKeysRepo) SetTemp(dc, idx int, key []byte, expires int64) error {
	if key == nil {
		_, err := r.delTempStmt.Exec(dc, idx)
		return err
	}
	_, err := r.setTempStmt.Exec(dc, idx, key, expires)
	return err
}

// GetTemp returns the temporary auth key for the given DC and index that
// has not yet expired (now < expires).
func (r *AuthKeysRepo) GetTemp(dc, idx int, now int64) ([]byte, error) {
	var key []byte
	err := r.getTempStmt.QueryRow(dc, idx, now).Scan(&key)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: temp_auth_keys.get dc=%d idx=%d: %w", dc, idx, err)
	}
	return key, nil
}

// DeleteByDc deletes all auth keys (permanent and temporary) for the given DC.
func (r *AuthKeysRepo) DeleteByDc(dc int) error {
	if _, err := r.delStmt.Exec(dc); err != nil {
		return err
	}
	_, err := r.delTempAllStmt.Exec(dc)
	return err
}

// DeleteAll deletes all auth keys.
func (r *AuthKeysRepo) DeleteAll() error {
	tx, err := r.drv.db.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: auth_keys.deleteAll: begin: %w", err)
	}
	if _, err := tx.Stmt(r.delAllStmt).Exec(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlite: auth_keys.deleteAll: delete permanent keys: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM temp_auth_keys`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlite: auth_keys.deleteAll: delete temporary keys: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: auth_keys.deleteAll: commit: %w", err)
	}
	return nil
}
