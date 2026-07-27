package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
)

// PeerInfo is the cached peer representation stored in the peers table.
type PeerInfo struct {
	ID         int64
	AccessHash string
	IsMin      bool
	Usernames  []string
	Updated    int64
	Phone      string // empty if unavailable
	Complete   []byte // serialized tl.TypeUser or tl.TypeChat
}

// PeersRepo caches peer information (users and chats) for fast lookup.
type PeersRepo struct {
	drv *driver

	storeStmt         *sql.Stmt
	getByIDStmt       *sql.Stmt
	getByUsernameStmt *sql.Stmt
	getByPhoneStmt    *sql.Stmt
	delAllStmt        *sql.Stmt
}

func newPeersRepo(drv *driver) *PeersRepo {
	r := &PeersRepo{drv: drv}
	drv.registerOnLoad(r.prepare)
	return r
}

func (r *PeersRepo) prepare() error {
	db := r.drv.db
	var err error

	r.storeStmt, err = db.Prepare(
		`INSERT OR REPLACE INTO peers (id, hash, is_min, usernames, updated, phone, complete)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare peers.store: %w", err)
	}
	r.getByIDStmt, err = db.Prepare(
		`SELECT id, hash, is_min, usernames, updated, phone, complete FROM peers WHERE id = ?`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare peers.getById: %w", err)
	}
	r.getByUsernameStmt, err = db.Prepare(
		`SELECT id, hash, is_min, usernames, updated, phone, complete FROM peers
		 WHERE is_min = 0 AND usernames LIKE ? LIMIT 1`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare peers.getByUsername: %w", err)
	}
	r.getByPhoneStmt, err = db.Prepare(
		`SELECT id, hash, is_min, usernames, updated, phone, complete FROM peers
		 WHERE is_min = 0 AND phone = ? LIMIT 1`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare peers.getByPhone: %w", err)
	}
	r.delAllStmt, err = db.Prepare(`DELETE FROM peers`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare peers.delAll: %w", err)
	}

	return nil
}

// Store saves or updates a cached peer.
func (r *PeersRepo) Store(peer PeerInfo) error {
	isMin := 0
	if peer.IsMin {
		isMin = 1
	}
	usernames := strings.Join(peer.Usernames, "\x00")
	phone := sql.NullString{}
	if peer.Phone != "" {
		phone.String = peer.Phone
		phone.Valid = true
	}
	_, err := r.storeStmt.Exec(peer.ID, peer.AccessHash, isMin, usernames, peer.Updated, phone, peer.Complete)
	return err
}

// GetByID returns a cached peer by its ID.
func (r *PeersRepo) GetByID(id int64) (*PeerInfo, error) {
	var p PeerInfo
	var isMin int
	var usernames string
	var phone sql.NullString
	err := r.getByIDStmt.QueryRow(id).Scan(
		&p.ID, &p.AccessHash, &isMin, &usernames, &p.Updated, &phone, &p.Complete,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: peers.getById id=%d: %w", id, err)
	}
	p.IsMin = isMin != 0
	if usernames != "" {
		p.Usernames = strings.Split(usernames, "\x00")
	}
	if phone.Valid {
		p.Phone = phone.String
	}
	return &p, nil
}

// GetByUsername finds a non-min peer whose usernames contain the given
// username. Returns nil if not found.
func (r *PeersRepo) GetByUsername(username string) (*PeerInfo, error) {
	pattern := "%\x00" + username + "\x00%"
	var p PeerInfo
	var isMin int
	var usernames string
	var phone sql.NullString
	err := r.getByUsernameStmt.QueryRow(pattern).Scan(
		&p.ID, &p.AccessHash, &isMin, &usernames, &p.Updated, &phone, &p.Complete,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: peers.getByUsername %q: %w", username, err)
	}
	p.IsMin = isMin != 0
	if usernames != "" {
		p.Usernames = strings.Split(usernames, "\x00")
	}
	if phone.Valid {
		p.Phone = phone.String
	}
	return &p, nil
}

// GetByPhone finds a non-min peer by phone number.
func (r *PeersRepo) GetByPhone(phone string) (*PeerInfo, error) {
	var p PeerInfo
	var isMin int
	var usernames string
	var phoneCol sql.NullString
	err := r.getByPhoneStmt.QueryRow(phone).Scan(
		&p.ID, &p.AccessHash, &isMin, &usernames, &p.Updated, &phoneCol, &p.Complete,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: peers.getByPhone %q: %w", phone, err)
	}
	p.IsMin = isMin != 0
	if usernames != "" {
		p.Usernames = strings.Split(usernames, "\x00")
	}
	if phoneCol.Valid {
		p.Phone = phoneCol.String
	}
	return &p, nil
}

// DeleteAll removes all cached peers.
func (r *PeersRepo) DeleteAll() error {
	_, err := r.delAllStmt.Exec()
	return err
}
