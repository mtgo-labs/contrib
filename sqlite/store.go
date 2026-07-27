package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/mtgo-labs/raw/session"
)

const sessionKey = "__mtgo_session__"

// Store persists session state in a SQLite database.
// It implements session.Store for use with raw.Client and exposes repository
// types (AuthKeys, KV, Peers, RefMessages) for direct access.
type Store struct {
	drv *driver

	AuthKeys    *AuthKeysRepo
	KV          *KVRepo
	Peers       *PeersRepo
	RefMessages *RefMessagesRepo
}

// NewStore opens or creates a SQLite store at path and applies schema
// migrations. The returned Store must be closed after use.
func NewStore(path string) (*Store, error) {
	drv, err := openDriver(path)
	if err != nil {
		return nil, err
	}

	s := &Store{drv: drv}

	// Repositories register their migrations and onLoad callbacks.
	s.AuthKeys = newAuthKeysRepo(drv)
	s.KV = newKVRepo(drv)
	s.Peers = newPeersRepo(drv)
	s.RefMessages = newRefMessagesRepo(drv)

	// Apply all registered migrations
	migrations := []migration{
		// auth_keys
		{repo: "auth_keys", version: 1, sql: `
			CREATE TABLE IF NOT EXISTS auth_keys (
				dc INTEGER PRIMARY KEY,
				key BLOB NOT NULL
			);
			CREATE TABLE IF NOT EXISTS temp_auth_keys (
				dc INTEGER NOT NULL,
				idx INTEGER NOT NULL,
				key BLOB NOT NULL,
				expires INTEGER NOT NULL,
				PRIMARY KEY (dc, idx)
			)`},
		// kv
		{repo: "kv", version: 1, sql: `
			CREATE TABLE IF NOT EXISTS key_value (
				key TEXT PRIMARY KEY,
				value BLOB NOT NULL
			)`},
		// ref_messages
		{repo: "ref_messages", version: 1, sql: `
			CREATE TABLE IF NOT EXISTS message_refs (
				peer_id INTEGER NOT NULL,
				chat_id INTEGER NOT NULL,
				msg_id INTEGER NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_message_refs_peer ON message_refs (peer_id);
			CREATE INDEX IF NOT EXISTS idx_message_refs ON message_refs (chat_id, msg_id)`},
		{repo: "ref_messages", version: 2, sql: `
			DELETE FROM message_refs
			WHERE rowid <> (
				SELECT newest.rowid
				FROM message_refs AS newest
				WHERE newest.peer_id = message_refs.peer_id
				ORDER BY newest.msg_id DESC, newest.rowid DESC
				LIMIT 1
			);
			DROP INDEX IF EXISTS idx_message_refs_peer;
			CREATE UNIQUE INDEX idx_message_refs_peer ON message_refs (peer_id)`},
		// peers
		{repo: "peers", version: 1, sql: `
			CREATE TABLE IF NOT EXISTS peers (
				id INTEGER PRIMARY KEY,
				hash TEXT NOT NULL,
				is_min INTEGER NOT NULL DEFAULT 0,
				usernames TEXT NOT NULL DEFAULT '',
				updated INTEGER NOT NULL DEFAULT 0,
				phone TEXT,
				complete BLOB NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_peers_username ON peers (usernames);
			CREATE INDEX IF NOT EXISTS idx_peers_phone ON peers (phone)`},
		{repo: "peers", version: 2, sql: `
			CREATE TABLE IF NOT EXISTS peer_usernames (
				peer_id INTEGER NOT NULL,
				username TEXT NOT NULL,
				PRIMARY KEY (peer_id, username)
			);
			CREATE INDEX IF NOT EXISTS idx_peer_usernames_username
				ON peer_usernames (username);
			DROP INDEX IF EXISTS idx_peers_username`,
			run: migratePeerUsernames},
	}

	if err := drv.applyMigrations(migrations); err != nil {
		drv.close()
		return nil, err
	}

	if err := drv.load(); err != nil {
		drv.close()
		return nil, err
	}

	return s, nil
}

// Load returns the stored session data. It implements session.Store.
func (s *Store) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := s.KV.Get(sessionKey)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, fmt.Errorf("%w: no session data", session.ErrSessionNotFound)
	}
	return data, nil
}

// Save persists session data. It implements session.Store.
func (s *Store) Save(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("%w: empty snapshot", errors.New("sqlite: empty session data"))
	}
	return s.KV.Set(sessionKey, data)
}

// Close closes the database connection and releases resources.
func (s *Store) Close() error {
	return s.drv.close()
}
