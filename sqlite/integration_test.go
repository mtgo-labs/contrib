package sqlite_test

import (
	"context"
	"testing"

	"github.com/mtgo-labs/contrib/sqlite"
	"github.com/mtgo-labs/raw/session"
)

// mkKey creates a valid 256-byte auth key (RSA-2048 size) for testing.
func mkKey(seed byte) []byte {
	k := make([]byte, 256)
	k[0] = seed
	return k
}

// TestStoreIntegration_Lifetime mirrors the raw client's usage: encode a
// session.Snapshot, save, close, reopen, load, and decode.
func TestStoreIntegration_Lifetime(t *testing.T) {
	path := t.TempDir() + "/session.db"

	// --- First session: create store, encode snapshot, save ---
	store1, err := sqlite.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := session.Snapshot{
		APIID:     12345,
		PrimaryDC: 2,
		SessionID: [8]byte{0, 1, 2, 3, 4, 5, 6, 7},
		AuthKeys: []session.AuthKey{
			{DCID: 2, Kind: "perm", Key: mkKey(1), ID: 0x1234, Salt: 100},
			{DCID: 4, Kind: "perm", Key: mkKey(2), ID: 0x5678, Salt: 200},
		},
	}
	data, err := session.Encode(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store1.Save(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := store1.Close(); err != nil {
		t.Fatal(err)
	}

	// --- Second session: reopen, load, decode ---
	store2, err := sqlite.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	data, err = store2.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := session.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.APIID != snapshot.APIID {
		t.Fatalf("APIID: expected %d, got %d", snapshot.APIID, decoded.APIID)
	}
	if decoded.PrimaryDC != snapshot.PrimaryDC {
		t.Fatalf("PrimaryDC: expected %d, got %d", snapshot.PrimaryDC, decoded.PrimaryDC)
	}
	if decoded.SessionID != snapshot.SessionID {
		t.Fatalf("SessionID mismatch")
	}
	if len(decoded.AuthKeys) != 2 {
		t.Fatalf("expected 2 auth keys, got %d", len(decoded.AuthKeys))
	}
	for i, key := range decoded.AuthKeys {
		if key.DCID != snapshot.AuthKeys[i].DCID {
			t.Fatalf("auth key %d DC: expected %d, got %d", i, snapshot.AuthKeys[i].DCID, key.DCID)
		}
		if string(key.Key) != string(snapshot.AuthKeys[i].Key) {
			t.Fatalf("auth key %d value mismatch", i)
		}
	}
}

// TestStoreIntegration_NewClientMimicry shows how an application would plug the
// SQLite store into a raw.Client. This mimics the real usage pattern.
func TestStoreIntegration_NewClientMimicry(t *testing.T) {
	path := t.TempDir() + "/session.db"
	store, err := sqlite.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// In a real application, this is:
	//
	//   client, err := raw.NewClient(raw.Config{
	//       APIID:    12345,
	//       APIHash:  "abc123",
	//       BotToken: "...",
	//       Store:    store,   // <-- SQLite store plugs in here
	//   })
	//
	// The client then calls store.Load() on startup and store.Save()
	// whenever auth state changes.

	// Verify the store is empty initially (no session exists).
	_, err = store.Load(context.Background())
	if err == nil {
		t.Fatal("expected error loading from empty store")
	}

	// After first login, the client persists auth state:
	snapshot := session.Snapshot{
		APIID:     9999,
		PrimaryDC: 3,
		SessionID: [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
		AuthKeys: []session.AuthKey{
			{DCID: 3, Kind: "perm", Key: mkKey(3), ID: 0xABCD, Salt: 300},
		},
	}
	data, _ := session.Encode(snapshot)
	if err := store.Save(context.Background(), data); err != nil {
		t.Fatal(err)
	}

	// On next startup, the client loads the saved state:
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) == 0 {
		t.Fatal("empty data loaded")
	}
}

// TestStoreIntegration_RepositoriesAlongsideSession shows that you can use
// the rich repository API alongside the opaque session store — auth keys,
// key-value pairs, peer cache, and message references are all available.
func TestStoreIntegration_RepositoriesAlongsideSession(t *testing.T) {
	path := t.TempDir() + "/session.db"
	store, err := sqlite.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// 1. Save session state (what the raw client does)
	snapshot := session.Snapshot{
		APIID:     1,
		PrimaryDC: 2,
		AuthKeys: []session.AuthKey{
			{DCID: 2, Kind: "perm", Key: mkKey(9), ID: 0x9999, Salt: 900},
		},
	}
	data, _ := session.Encode(snapshot)
	if err := store.Save(ctx, data); err != nil {
		t.Fatal(err)
	}

	// 2. Store additional auth keys via repository (alternative to snapshot.AuthKeys)
	if err := store.AuthKeys.Set(2, []byte("dc2-key")); err != nil {
		t.Fatal(err)
	}
	if err := store.AuthKeys.SetTemp(2, 0, []byte("temp-dc2"), 99999); err != nil {
		t.Fatal(err)
	}

	// 3. Store app-level key-value data
	store.KV.Set("last_dc", []byte("2"))
	store.KV.Set("user_id", []byte("123456"))

	// 4. Cache peers
	store.Peers.Store(sqlite.PeerInfo{
		ID:         123456,
		AccessHash: "hash123",
		Usernames:  []string{"testuser"},
		Complete:   []byte("serialized-user"),
	})

	// 5. Store message references
	store.RefMessages.Store(123456, 789012, 42)

	// Verify all persists across reopen
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store2, err := sqlite.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	// Session still loads
	loaded, err := store2.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := session.Decode(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.PrimaryDC != 2 {
		t.Fatal("session not persisted")
	}

	// Auth keys survive
	key, err := store2.AuthKeys.Get(2)
	if err != nil || string(key) != "dc2-key" {
		t.Fatal("auth key not persisted")
	}
	tempKey, err := store2.AuthKeys.GetTemp(2, 0, 500)
	if err != nil || string(tempKey) != "temp-dc2" {
		t.Fatal("temp auth key not persisted")
	}

	// KV survives
	v, err := store2.KV.Get("last_dc")
	if err != nil || string(v) != "2" {
		t.Fatal("kv not persisted")
	}

	// Peer survives
	peer, err := store2.Peers.GetByID(123456)
	if err != nil || peer == nil || peer.Usernames[0] != "testuser" {
		t.Fatal("peer not persisted")
	}

	// Ref messages survive
	chatID, msgID, found, _ := store2.RefMessages.GetByPeer(123456)
	if !found || chatID != 789012 || msgID != 42 {
		t.Fatal("ref message not persisted")
	}
}
