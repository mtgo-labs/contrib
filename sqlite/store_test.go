package sqlite

import (
	"context"
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
	if err := store.AuthKeys.DeleteAll(); err != nil {
		t.Fatal(err)
	}

	for _, dc := range []int{2, 4} {
		got, _ := store.AuthKeys.Get(dc)
		if got != nil {
			t.Fatalf("expected nil for dc %d", dc)
		}
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

	chatID, msgID, found, err := store.RefMessages.GetByPeer(100)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected found")
	}
	if chatID != 200 || msgID != 300 {
		t.Fatalf("expected (200, 300), got (%d, %d)", chatID, msgID)
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
