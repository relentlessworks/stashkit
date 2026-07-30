package store

import (
	"os"
	"testing"
	"time"

	"github.com/relentlessworks/stashkit/internal/model"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "stashkit-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateWorkspace(t *testing.T) {
	s := tempStore(t)
	ws, err := s.CreateWorkspace("test-ws")
	if err != nil {
		t.Fatalf("CreateWorkspace failed: %v", err)
	}
	if ws.Handle == "" {
		t.Error("expected non-empty handle")
	}
	if ws.Name != "test-ws" {
		t.Errorf("expected name 'test-ws', got '%s'", ws.Name)
	}
	if ws.Plan != "free" {
		t.Errorf("expected plan 'free', got '%s'", ws.Plan)
	}

	// Verify it can be retrieved
	got, ok := s.GetWorkspace(ws.Handle)
	if !ok {
		t.Error("workspace not found after creation")
	}
	if got.Name != "test-ws" {
		t.Errorf("expected name 'test-ws', got '%s'", got.Name)
	}
}

func TestListWorkspaces(t *testing.T) {
	s := tempStore(t)
	s.CreateWorkspace("ws1")
	s.CreateWorkspace("ws2")
	s.CreateWorkspace("ws3")

	list := s.ListWorkspaces()
	if len(list) != 3 {
		t.Errorf("expected 3 workspaces, got %d", len(list))
	}
}

func TestPutAndGetEntry(t *testing.T) {
	s := tempStore(t)
	ws, _ := s.CreateWorkspace("test")

	entry, err := s.PutEntry(ws.Handle, "mykey", "myvalue", "", nil)
	if err != nil {
		t.Fatalf("PutEntry failed: %v", err)
	}
	if entry.Handle == "" {
		t.Error("expected non-empty handle")
	}
	if entry.Key != "mykey" {
		t.Errorf("expected key 'mykey', got '%s'", entry.Key)
	}
	if entry.Value != "myvalue" {
		t.Errorf("expected value 'myvalue', got '%s'", entry.Value)
	}

	// Get by handle
	got, ok := s.GetEntry(ws.Handle, entry.Handle)
	if !ok {
		t.Error("entry not found by handle")
	}
	if got.Value != "myvalue" {
		t.Errorf("expected value 'myvalue', got '%s'", got.Value)
	}

	// Get by key
	gotByKey, ok := s.GetEntryByKey(ws.Handle, "mykey", "")
	if !ok {
		t.Error("entry not found by key")
	}
	if gotByKey.Value != "myvalue" {
		t.Errorf("expected value 'myvalue', got '%s'", gotByKey.Value)
	}
}

func TestPutEntryUpdatesExisting(t *testing.T) {
	s := tempStore(t)
	ws, _ := s.CreateWorkspace("test")

	entry1, _ := s.PutEntry(ws.Handle, "key1", "value1", "", nil)
	entry2, _ := s.PutEntry(ws.Handle, "key1", "value2", "", nil)

	if entry1.Handle != entry2.Handle {
		t.Error("expected same handle for same key (update, not create)")
	}
	if entry2.Value != "value2" {
		t.Errorf("expected updated value 'value2', got '%s'", entry2.Value)
	}

	// Should only have 1 entry
	entries := s.ListEntries(ws.Handle, "")
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestPutEntryWithNamespace(t *testing.T) {
	s := tempStore(t)
	ws, _ := s.CreateWorkspace("test")

	// Same key in different namespaces should be separate entries
	s.PutEntry(ws.Handle, "key1", "value_ns1", "ns1", nil)
	s.PutEntry(ws.Handle, "key1", "value_ns2", "ns2", nil)

	entries := s.ListEntries(ws.Handle, "")
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Filter by namespace
	ns1Entries := s.ListEntries(ws.Handle, "ns1")
	if len(ns1Entries) != 1 {
		t.Errorf("expected 1 entry in ns1, got %d", len(ns1Entries))
	}
	if ns1Entries[0].Value != "value_ns1" {
		t.Errorf("expected 'value_ns1', got '%s'", ns1Entries[0].Value)
	}
}

func TestEntryWithTTL(t *testing.T) {
	s := tempStore(t)
	ws, _ := s.CreateWorkspace("test")

	ttl := 100 * time.Millisecond
	entry, _ := s.PutEntry(ws.Handle, "tempkey", "tempvalue", "", &ttl)

	if entry.ExpiresAt == nil {
		t.Error("expected non-nil ExpiresAt")
	}

	// Should be retrievable immediately
	_, ok := s.GetEntry(ws.Handle, entry.Handle)
	if !ok {
		t.Error("entry should be retrievable before TTL expires")
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	_, ok = s.GetEntry(ws.Handle, entry.Handle)
	if ok {
		t.Error("entry should be expired after TTL")
	}

	// Also check by key
	_, ok = s.GetEntryByKey(ws.Handle, "tempkey", "")
	if ok {
		t.Error("entry should be expired by key after TTL")
	}
}

func TestDeleteEntry(t *testing.T) {
	s := tempStore(t)
	ws, _ := s.CreateWorkspace("test")

	entry, _ := s.PutEntry(ws.Handle, "key1", "value1", "", nil)

	err := s.DeleteEntry(ws.Handle, entry.Handle)
	if err != nil {
		t.Fatalf("DeleteEntry failed: %v", err)
	}

	_, ok := s.GetEntry(ws.Handle, entry.Handle)
	if ok {
		t.Error("entry should be deleted")
	}
}

func TestDeleteEntryByKey(t *testing.T) {
	s := tempStore(t)
	ws, _ := s.CreateWorkspace("test")

	s.PutEntry(ws.Handle, "key1", "value1", "", nil)

	err := s.DeleteEntryByKey(ws.Handle, "key1", "")
	if err != nil {
		t.Fatalf("DeleteEntryByKey failed: %v", err)
	}

	_, ok := s.GetEntryByKey(ws.Handle, "key1", "")
	if ok {
		t.Error("entry should be deleted by key")
	}
}

func TestListEntries(t *testing.T) {
	s := tempStore(t)
	ws, _ := s.CreateWorkspace("test")

	s.PutEntry(ws.Handle, "key1", "value1", "", nil)
	s.PutEntry(ws.Handle, "key2", "value2", "", nil)
	s.PutEntry(ws.Handle, "key3", "value3", "ns1", nil)

	all := s.ListEntries(ws.Handle, "")
	if len(all) != 3 {
		t.Errorf("expected 3 entries, got %d", len(all))
	}

	ns1Only := s.ListEntries(ws.Handle, "ns1")
	if len(ns1Only) != 1 {
		t.Errorf("expected 1 entry in ns1, got %d", len(ns1Only))
	}
}

func TestCountEntries(t *testing.T) {
	s := tempStore(t)
	ws, _ := s.CreateWorkspace("test")

	s.PutEntry(ws.Handle, "key1", "value1", "", nil)
	s.PutEntry(ws.Handle, "key2", "value2", "", nil)

	count := s.CountEntries(ws.Handle)
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestAuditLog(t *testing.T) {
	s := tempStore(t)
	ws, _ := s.CreateWorkspace("test")

	s.AddAuditLog(ws.Handle, "entry.put", "key=test", "user@example.com")
	s.AddAuditLog(ws.Handle, "entry.delete", "key=test", "user@example.com")

	logs := s.ListAuditLogs(ws.Handle)
	if len(logs) != 2 {
		t.Errorf("expected 2 audit logs, got %d", len(logs))
	}
	if logs[0].Action != "entry.put" {
		t.Errorf("expected first action 'entry.put', got '%s'", logs[0].Action)
	}
	if logs[1].Action != "entry.delete" {
		t.Errorf("expected second action 'entry.delete', got '%s'", logs[1].Action)
	}
}

func TestPersistence(t *testing.T) {
	dir, err := os.MkdirTemp("", "stashkit-persist-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Create store, add data
	s1, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := s1.CreateWorkspace("persist-test")
	s1.PutEntry(ws.Handle, "key1", "value1", "", nil)
	s1.AddAuditLog(ws.Handle, "test", "persistence test", "tester")

	// Create new store from same directory
	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify workspace persisted
	gotWs, ok := s2.GetWorkspace(ws.Handle)
	if !ok {
		t.Error("workspace not persisted")
	}
	if gotWs.Name != "persist-test" {
		t.Errorf("expected name 'persist-test', got '%s'", gotWs.Name)
	}

	// Verify entry persisted
	gotEntry, ok := s2.GetEntryByKey(ws.Handle, "key1", "")
	if !ok {
		t.Error("entry not persisted")
	}
	if gotEntry.Value != "value1" {
		t.Errorf("expected value 'value1', got '%s'", gotEntry.Value)
	}

	// Verify audit log persisted
	logs := s2.ListAuditLogs(ws.Handle)
	if len(logs) != 1 {
		t.Errorf("expected 1 audit log, got %d", len(logs))
	}
}

func TestHandleGeneration(t *testing.T) {
	h1 := model.NewStashHandle()
	h2 := model.NewStashHandle()

	if h1 == h2 {
		t.Error("expected different handles")
	}
	if len(h1) < 11 { // "stash_" + 5 chars
		t.Errorf("handle too short: %s", h1)
	}
}
