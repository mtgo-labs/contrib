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

	storeStmt           *sql.Stmt
	storeUsernameStmt   *sql.Stmt
	delUsernamesStmt    *sql.Stmt
	getByIDStmt         *sql.Stmt
	getByUsernameStmt   *sql.Stmt
	getByPhoneStmt      *sql.Stmt
	delAllStmt          *sql.Stmt
	delAllUsernamesStmt *sql.Stmt
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
		`INSERT INTO peers (id, hash, is_min, usernames, updated, phone, complete)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			hash = excluded.hash,
			is_min = excluded.is_min,
			usernames = excluded.usernames,
			updated = excluded.updated,
			phone = excluded.phone,
			complete = excluded.complete`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare peers.store: %w", err)
	}
	r.storeUsernameStmt, err = db.Prepare(
		`INSERT OR IGNORE INTO peer_usernames (peer_id, username) VALUES (?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare peer_usernames.store: %w", err)
	}
	r.delUsernamesStmt, err = db.Prepare(`DELETE FROM peer_usernames WHERE peer_id = ?`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare peer_usernames.delete: %w", err)
	}
	r.getByIDStmt, err = db.Prepare(
		`SELECT id, hash, is_min, usernames, updated, phone, complete FROM peers WHERE id = ?`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare peers.getById: %w", err)
	}
	r.getByUsernameStmt, err = db.Prepare(
		`SELECT p.id, p.hash, p.is_min, p.usernames, p.updated, p.phone, p.complete
		 FROM peer_usernames AS u
		 JOIN peers AS p ON p.id = u.peer_id
		 WHERE p.is_min = 0 AND u.username = ?
		 LIMIT 1`,
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
	r.delAllUsernamesStmt, err = db.Prepare(`DELETE FROM peer_usernames`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare peer_usernames.delAll: %w", err)
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

	tx, err := r.drv.db.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: peers.store id=%d: begin: %w", peer.ID, err)
	}
	if _, err := tx.Stmt(r.storeStmt).Exec(
		peer.ID, peer.AccessHash, isMin, usernames, peer.Updated, phone, peer.Complete,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlite: peers.store id=%d: %w", peer.ID, err)
	}
	if _, err := tx.Stmt(r.delUsernamesStmt).Exec(peer.ID); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlite: peers.store id=%d: clear usernames: %w", peer.ID, err)
	}
	for _, username := range peer.Usernames {
		username = strings.ToLower(username)
		if username == "" {
			continue
		}
		if _, err := tx.Stmt(r.storeUsernameStmt).Exec(peer.ID, username); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite: peers.store id=%d: store username %q: %w", peer.ID, username, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: peers.store id=%d: commit: %w", peer.ID, err)
	}
	return nil
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

// GetByUsername finds a non-min peer with the given exact username.
// Returns nil if not found.
func (r *PeersRepo) GetByUsername(username string) (*PeerInfo, error) {
	var p PeerInfo
	var isMin int
	var usernames string
	var phone sql.NullString
	err := r.getByUsernameStmt.QueryRow(strings.ToLower(username)).Scan(
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
	tx, err := r.drv.db.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: peers.delAll: begin: %w", err)
	}
	if _, err := tx.Stmt(r.delAllUsernamesStmt).Exec(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlite: peers.delAll: delete usernames: %w", err)
	}
	if _, err := tx.Stmt(r.delAllStmt).Exec(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlite: peers.delAll: delete peers: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: peers.delAll: commit: %w", err)
	}
	return nil
}

func migratePeerUsernames(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, usernames FROM peers`)
	if err != nil {
		return fmt.Errorf("query legacy peers: %w", err)
	}

	type legacyPeer struct {
		id        int64
		usernames string
	}
	var peers []legacyPeer
	for rows.Next() {
		var peer legacyPeer
		if err := rows.Scan(&peer.id, &peer.usernames); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy peer: %w", err)
		}
		peers = append(peers, peer)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate legacy peers: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close legacy peers: %w", err)
	}

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO peer_usernames (peer_id, username) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare peer username backfill: %w", err)
	}
	defer stmt.Close()

	for _, peer := range peers {
		if peer.usernames == "" {
			continue
		}
		for _, username := range strings.Split(peer.usernames, "\x00") {
			username = strings.ToLower(username)
			if username == "" {
				continue
			}
			if _, err := stmt.Exec(peer.id, username); err != nil {
				return fmt.Errorf("backfill peer %d username %q: %w", peer.id, username, err)
			}
		}
	}
	return nil
}
