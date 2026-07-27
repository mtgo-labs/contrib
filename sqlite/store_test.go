package sqlite

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/mtgo-labs/raw/session"
)

func TestStoreImplementsSessionStore(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var _ session.Store = store
}

func TestStoreSaveAndLoad(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	input := []byte("session-state")
	if err := store.Save(ctx, input); err != nil {
		t.Fatal(err)
	}

	output, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != string(input) {
		t.Fatalf("expected %q, got %q", input, output)
	}
}

func TestStoreSaveReturnsWriteFailure(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if err := store.Save(context.Background(), []byte("data")); err == nil {
		t.Fatal("expected closed database write to fail")
	}
}

func TestStoreLoadNotFound(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.Load(ctx)
	if err == nil {
		t.Fatal("expected error for empty store")
	}
}

func TestStoreRejectEmptySave(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.Save(ctx, nil); err == nil {
		t.Fatal("expected error for empty save")
	}
}

func TestStoreHonorsCancellation(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.Save(ctx, []byte("x")); err == nil {
		t.Fatal("expected error for cancelled save")
	}
	if _, err := store.Load(ctx); err == nil {
		t.Fatal("expected error for cancelled load")
	}
}

func TestStorePersistenceAcrossOpens(t *testing.T) {
	path := testDBPath(t)

	store1, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store1.Save(context.Background(), []byte("persisted")); err != nil {
		t.Fatal(err)
	}
	if err := store1.Close(); err != nil {
		t.Fatal(err)
	}

	store2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	data, err := store2.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "persisted" {
		t.Fatalf("expected 'persisted', got %q", data)
	}
}

func TestAuthKeysSetAndGet(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	key := []byte("auth-key-dc-2")
	if err := store.AuthKeys.Set(2, key); err != nil {
		t.Fatal(err)
	}

	got, err := store.AuthKeys.Get(2)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(key) {
		t.Fatalf("expected %q, got %q", key, got)
	}
}

func TestAuthKeysGetNotFound(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got, err := store.AuthKeys.Get(5)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil for missing key")
	}
}

func TestAuthKeysSetNilDeletes(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.AuthKeys.Set(2, []byte("temp")); err != nil {
		t.Fatal(err)
	}
	if err := store.AuthKeys.Set(2, nil); err != nil {
		t.Fatal(err)
	}

	got, err := store.AuthKeys.Get(2)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestAuthKeysTempKeys(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	key := []byte("temp-auth")
	expires := int64(1000)
	if err := store.AuthKeys.SetTemp(2, 0, key, expires); err != nil {
		t.Fatal(err)
	}

	// Not expired
	got, err := store.AuthKeys.GetTemp(2, 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(key) {
		t.Fatalf("expected %q, got %q", key, got)
	}

	// Expired
	got, err = store.AuthKeys.GetTemp(2, 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil for expired key")
	}
}

func TestAuthKeysDeleteByDc(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.AuthKeys.Set(2, []byte("perm")); err != nil {
		t.Fatal(err)
	}
	if err := store.AuthKeys.SetTemp(2, 0, []byte("temp"), 10000); err != nil {
		t.Fatal(err)
	}
	if err := store.AuthKeys.DeleteByDc(2); err != nil {
		t.Fatal(err)
	}

	got, _ := store.AuthKeys.Get(2)
	if got != nil {
		t.Fatal("expected nil after deleteByDc")
	}
	got, _ = store.AuthKeys.GetTemp(2, 0, 500)
	if got != nil {
		t.Fatal("expected nil after deleteByDc")
	}
}

func TestAuthKeysDeleteAll(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.AuthKeys.Set(2, []byte("k2")); err != nil {
		t.Fatal(err)
	}
	if err := store.AuthKeys.Set(4, []byte("k4")); err != nil {
		t.Fatal(err)
	}
	if err := store.AuthKeys.SetTemp(2, 0, []byte("temp"), 10000); err != nil {
		t.Fatal(err)
	}
	if err := store.AuthKeys.DeleteAll(); err != nil {
		t.Fatal(err)
	}

	for _, dc := range []int{2, 4} {
		got, _ := store.AuthKeys.Get(dc)
		if got != nil {
			t.Fatalf("expected nil for dc %d", dc)
		}
	}
	got, _ := store.AuthKeys.GetTemp(2, 0, 500)
	if got != nil {
		t.Fatal("expected temporary keys to be deleted")
	}
}

func TestAuthKeysDeleteAllIsAtomic(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.AuthKeys.Set(2, []byte("permanent")); err != nil {
		t.Fatal(err)
	}
	if err := store.AuthKeys.SetTemp(2, 0, []byte("temporary"), 10000); err != nil {
		t.Fatal(err)
	}
	if _, err := store.drv.db.Exec(`
		CREATE TRIGGER reject_temp_auth_key_delete
		BEFORE DELETE ON temp_auth_keys
		BEGIN
			SELECT RAISE(ABORT, 'delete rejected');
		END
	`); err != nil {
		t.Fatal(err)
	}

	if err := store.AuthKeys.DeleteAll(); err == nil {
		t.Fatal("expected delete to fail")
	}
	permanent, err := store.AuthKeys.Get(2)
	if err != nil {
		t.Fatal(err)
	}
	if string(permanent) != "permanent" {
		t.Fatalf("permanent key was not rolled back: %q", permanent)
	}
	temporary, err := store.AuthKeys.GetTemp(2, 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if string(temporary) != "temporary" {
		t.Fatalf("temporary key changed: %q", temporary)
	}
}

func TestKVSetAndGet(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.KV.Set("foo", []byte("bar"))

	got, err := store.KV.Get("foo")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "bar" {
		t.Fatalf("expected 'bar', got %q", got)
	}
}

func TestKVGetNotFound(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got, err := store.KV.Get("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil")
	}
}

func TestKVDelete(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.KV.Set("del", []byte("x"))
	if err := store.KV.Delete("del"); err != nil {
		t.Fatal(err)
	}

	got, _ := store.KV.Get("del")
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestKVDeleteAll(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.KV.Set("a", []byte("1"))
	store.KV.Set("b", []byte("2"))
	if err := store.KV.DeleteAll(); err != nil {
		t.Fatal(err)
	}

	for _, k := range []string{"a", "b"} {
		got, _ := store.KV.Get(k)
		if got != nil {
			t.Fatalf("expected nil for key %q", k)
		}
	}
}

func TestPeersStoreAndGetByID(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	peer := PeerInfo{
		ID:         12345,
		AccessHash: "abc123",
		IsMin:      false,
		Usernames:  []string{"testuser"},
		Updated:    1000,
		Phone:      "+1234567890",
		Complete:   []byte("serialized-peer"),
	}
	if err := store.Peers.Store(peer); err != nil {
		t.Fatal(err)
	}

	got, err := store.Peers.GetByID(12345)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected peer")
	}
	if got.ID != peer.ID || got.AccessHash != peer.AccessHash {
		t.Fatalf("mismatch: %+v vs %+v", got, peer)
	}
	if string(got.Complete) != string(peer.Complete) {
		t.Fatal("complete mismatch")
	}
}

func TestPeersGetByIDNotFound(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got, err := store.Peers.GetByID(999)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil")
	}
}

func TestPeersGetByUsername(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	peer := PeerInfo{
		ID:         42,
		AccessHash: "hash42",
		Usernames:  []string{"alice", "alice_work"},
		Complete:   []byte("data"),
	}
	if err := store.Peers.Store(peer); err != nil {
		t.Fatal(err)
	}

	got, err := store.Peers.GetByUsername("alice")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 42 {
		t.Fatalf("expected peer 42, got %v", got)
	}

	got, err = store.Peers.GetByUsername("ALICE_WORK")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 42 {
		t.Fatalf("expected case-insensitive exact match for peer 42, got %v", got)
	}

	got, err = store.Peers.GetByUsername("alice_wor")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("partial username unexpectedly matched peer %d", got.ID)
	}
}

func TestPeersStoreUpdatesUsernamesAtomically(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	original := PeerInfo{
		ID:         42,
		AccessHash: "original",
		Usernames:  []string{"alice"},
		Complete:   []byte("original"),
	}
	if err := store.Peers.Store(original); err != nil {
		t.Fatal(err)
	}
	if _, err := store.drv.db.Exec(`
		CREATE TRIGGER reject_blocked_username
		BEFORE INSERT ON peer_usernames
		WHEN NEW.username = 'blocked'
		BEGIN
			SELECT RAISE(ABORT, 'username rejected');
		END
	`); err != nil {
		t.Fatal(err)
	}

	updated := original
	updated.AccessHash = "updated"
	updated.Usernames = []string{"blocked"}
	updated.Complete = []byte("updated")
	if err := store.Peers.Store(updated); err == nil {
		t.Fatal("expected peer update to fail")
	}

	got, err := store.Peers.GetByID(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.AccessHash != original.AccessHash {
		t.Fatalf("peer row was not rolled back: %+v", got)
	}
	got, err = store.Peers.GetByUsername("alice")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != original.ID {
		t.Fatalf("original username was not rolled back: %+v", got)
	}
}

func TestPeersMigrationBackfillsUsernames(t *testing.T) {
	path := testDBPath(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE mtgo_migrations (
			repo TEXT NOT NULL PRIMARY KEY,
			version INTEGER NOT NULL
		);
		CREATE TABLE peers (
			id INTEGER PRIMARY KEY,
			hash TEXT NOT NULL,
			is_min INTEGER NOT NULL DEFAULT 0,
			usernames TEXT NOT NULL DEFAULT '',
			updated INTEGER NOT NULL DEFAULT 0,
			phone TEXT,
			complete BLOB NOT NULL
		);
		CREATE INDEX idx_peers_username ON peers (usernames);
		INSERT INTO mtgo_migrations (repo, version) VALUES ('peers', 1)
	`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO peers (id, hash, usernames, complete) VALUES (?, ?, ?, ?)`,
		42, "hash42", "Alice\x00alice_work", []byte("data"),
	); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, username := range []string{"alice", "ALICE_WORK"} {
		got, err := store.Peers.GetByUsername(username)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.ID != 42 {
			t.Fatalf("backfilled username %q was not searchable: %+v", username, got)
		}
	}
}

func TestPeersGetByPhone(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	peer := PeerInfo{
		ID:         99,
		AccessHash: "h99",
		Phone:      "+999",
		Complete:   []byte("data"),
	}
	if err := store.Peers.Store(peer); err != nil {
		t.Fatal(err)
	}

	got, err := store.Peers.GetByPhone("+999")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 99 {
		t.Fatalf("expected peer 99, got %v", got)
	}
}

func TestPeersDeleteAll(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Peers.Store(PeerInfo{ID: 1, Complete: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if err := store.Peers.DeleteAll(); err != nil {
		t.Fatal(err)
	}

	got, _ := store.Peers.GetByID(1)
	if got != nil {
		t.Fatal("expected nil after deleteAll")
	}
}

func TestRefMessagesStoreAndGet(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.RefMessages.Store(100, 200, 300); err != nil {
		t.Fatal(err)
	}
	if err := store.RefMessages.Store(100, 201, 301); err != nil {
		t.Fatal(err)
	}

	chatID, msgID, found, err := store.RefMessages.GetByPeer(100)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected found")
	}
	if chatID != 201 || msgID != 301 {
		t.Fatalf("expected latest reference (201, 301), got (%d, %d)", chatID, msgID)
	}
	var count int
	if err := store.drv.db.QueryRow(
		`SELECT COUNT(*) FROM message_refs WHERE peer_id = ?`, 100,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one reference for peer, got %d", count)
	}
}

func TestRefMessagesMigrationKeepsHighestLatestRow(t *testing.T) {
	path := testDBPath(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE mtgo_migrations (
			repo TEXT NOT NULL PRIMARY KEY,
			version INTEGER NOT NULL
		);
		CREATE TABLE message_refs (
			peer_id INTEGER NOT NULL,
			chat_id INTEGER NOT NULL,
			msg_id INTEGER NOT NULL
		);
		CREATE INDEX idx_message_refs_peer ON message_refs (peer_id);
		CREATE INDEX idx_message_refs ON message_refs (chat_id, msg_id);
		INSERT INTO mtgo_migrations (repo, version) VALUES ('ref_messages', 1);
		INSERT INTO message_refs (peer_id, chat_id, msg_id) VALUES (100, 200, 299);
		INSERT INTO message_refs (peer_id, chat_id, msg_id) VALUES (100, 201, 300);
		INSERT INTO message_refs (peer_id, chat_id, msg_id) VALUES (100, 202, 300)
	`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	chatID, msgID, found, err := store.RefMessages.GetByPeer(100)
	if err != nil {
		t.Fatal(err)
	}
	if !found || chatID != 202 || msgID != 300 {
		t.Fatalf("expected latest highest reference (202, 300), got (%d, %d, %v)", chatID, msgID, found)
	}
	var count int
	if err := store.drv.db.QueryRow(
		`SELECT COUNT(*) FROM message_refs WHERE peer_id = ?`, 100,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one migrated reference, got %d", count)
	}
}

func TestRefMessagesGetByPeerNotFound(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, _, found, err := store.RefMessages.GetByPeer(999)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected not found")
	}
}

func TestRefMessagesDelete(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.RefMessages.Store(100, 200, 300); err != nil {
		t.Fatal(err)
	}
	if err := store.RefMessages.Store(100, 200, 301); err != nil {
		t.Fatal(err)
	}
	if err := store.RefMessages.Delete(200, []int64{300, 301}); err != nil {
		t.Fatal(err)
	}

	_, _, found, _ := store.RefMessages.GetByPeer(100)
	if found {
		t.Fatal("expected not found after delete")
	}
}

func TestRefMessagesDeleteByPeer(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.RefMessages.Store(1, 10, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.RefMessages.Store(2, 20, 2); err != nil {
		t.Fatal(err)
	}
	if err := store.RefMessages.DeleteByPeer(1); err != nil {
		t.Fatal(err)
	}

	_, _, found, _ := store.RefMessages.GetByPeer(1)
	if found {
		t.Fatal("expected not found for peer 1")
	}
	_, _, found, _ = store.RefMessages.GetByPeer(2)
	if !found {
		t.Fatal("expected found for peer 2")
	}
}

func TestRefMessagesDeleteAll(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.RefMessages.Store(1, 10, 1)
	store.RefMessages.Store(2, 20, 2)
	if err := store.RefMessages.DeleteAll(); err != nil {
		t.Fatal(err)
	}

	for _, pid := range []int64{1, 2} {
		_, _, found, _ := store.RefMessages.GetByPeer(pid)
		if found {
			t.Fatalf("expected not found for peer %d", pid)
		}
	}
}

func TestDriverLoadsMigrationVersions(t *testing.T) {
	path := testDBPath(t)
	runs := 0
	m := migration{
		repo:    "test",
		version: 1,
		run: func(*sql.Tx) error {
			runs++
			return nil
		},
	}

	drv, err := openDriver(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := drv.applyMigrations([]migration{m}); err != nil {
		drv.close()
		t.Fatal(err)
	}
	if err := drv.close(); err != nil {
		t.Fatal(err)
	}

	drv, err = openDriver(path)
	if err != nil {
		t.Fatal(err)
	}
	defer drv.close()
	if err := drv.applyMigrations([]migration{m}); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("migration ran %d times", runs)
	}
}

func TestDriverRollsBackMigrationAndVersionTogether(t *testing.T) {
	drv, err := openDriver(testDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer drv.close()

	err = drv.applyMigrations([]migration{{
		repo:    "rollback",
		version: 1,
		sql:     `CREATE TABLE rolled_back (id INTEGER PRIMARY KEY)`,
		run: func(*sql.Tx) error {
			return sql.ErrTxDone
		},
	}})
	if err == nil {
		t.Fatal("expected migration to fail")
	}

	var count int
	if err := drv.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'rolled_back'`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("migration schema was not rolled back")
	}
	if err := drv.db.QueryRow(
		`SELECT COUNT(*) FROM mtgo_migrations WHERE repo = 'rollback'`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration version was recorded")
	}
}

func testDBPath(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/test.db"
}

func TestFileRemovedOnClose(t *testing.T) {
	path := testDBPath(t)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("db file should still exist after close")
	}
}
