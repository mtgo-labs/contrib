package sqlite

import (
	"database/sql"
	"fmt"
)

// RefMessagesRepo stores message references for forwarding by peer.
type RefMessagesRepo struct {
	drv *driver

	storeStmt    *sql.Stmt
	getByPeerStmt *sql.Stmt
	delStmt      *sql.Stmt
	delByPeerStmt *sql.Stmt
	delAllStmt   *sql.Stmt
}

func newRefMessagesRepo(drv *driver) *RefMessagesRepo {
	r := &RefMessagesRepo{drv: drv}
	drv.registerOnLoad(r.prepare)
	return r
}

func (r *RefMessagesRepo) prepare() error {
	db := r.drv.db
	var err error

	r.storeStmt, err = db.Prepare(
		`INSERT OR REPLACE INTO message_refs (peer_id, chat_id, msg_id) VALUES (?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare ref_messages.store: %w", err)
	}
	r.getByPeerStmt, err = db.Prepare(
		`SELECT chat_id, msg_id FROM message_refs WHERE peer_id = ? ORDER BY msg_id DESC LIMIT 1`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare ref_messages.getByPeer: %w", err)
	}
	r.delStmt, err = db.Prepare(
		`DELETE FROM message_refs WHERE chat_id = ? AND msg_id = ?`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare ref_messages.del: %w", err)
	}
	r.delByPeerStmt, err = db.Prepare(
		`DELETE FROM message_refs WHERE peer_id = ?`,
	)
	if err != nil {
		return fmt.Errorf("sqlite: prepare ref_messages.delByPeer: %w", err)
	}
	r.delAllStmt, err = db.Prepare(`DELETE FROM message_refs`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare ref_messages.delAll: %w", err)
	}

	return nil
}

// Store saves a message reference for forwarding.
func (r *RefMessagesRepo) Store(peerID, chatID, msgID int64) error {
	_, err := r.storeStmt.Exec(peerID, chatID, msgID)
	return err
}

// GetByPeer returns the (chat_id, msg_id) reference for the given peer,
// preferring the highest msg_id. Returns nil if none found.
func (r *RefMessagesRepo) GetByPeer(peerID int64) (chatID, msgID int64, found bool, _ error) {
	err := r.getByPeerStmt.QueryRow(peerID).Scan(&chatID, &msgID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, false, nil
		}
		return 0, 0, false, fmt.Errorf("sqlite: ref_messages.getByPeer peer=%d: %w", peerID, err)
	}
	return chatID, msgID, true, nil
}

// Delete removes references for the given chat where msgID matches.
func (r *RefMessagesRepo) Delete(chatID int64, msgIDs []int64) error {
	for _, msgID := range msgIDs {
		if _, err := r.delStmt.Exec(chatID, msgID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteByPeer removes all message references for the given peer.
func (r *RefMessagesRepo) DeleteByPeer(peerID int64) error {
	_, err := r.delByPeerStmt.Exec(peerID)
	return err
}

// DeleteAll removes all message references.
func (r *RefMessagesRepo) DeleteAll() error {
	_, err := r.delAllStmt.Exec()
	return err
}
