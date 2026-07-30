package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/relentlessworks/stashkit/internal/model"
)

// Store manages all data persistence using JSON files.
type Store struct {
	mu        sync.RWMutex
	dataDir   string
	workspaces map[string]*model.Workspace
	entries   map[string]map[string]*model.Entry // workspace -> handle -> entry
	auditLogs map[string][]model.AuditLog        // workspace -> logs
	nextAuditID int
}

// New creates a new store with the given data directory.
func New(dataDir string) (*Store, error) {
	s := &Store{
		dataDir:    dataDir,
		workspaces: make(map[string]*model.Workspace),
		entries:    make(map[string]map[string]*model.Entry),
		auditLogs:  make(map[string][]model.AuditLog),
		nextAuditID: 1,
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load data: %w", err)
	}

	return s, nil
}

// load reads all JSON files from the data directory.
func (s *Store) load() error {
	// Load workspaces
	wsPath := filepath.Join(s.dataDir, "workspaces.json")
	if data, err := os.ReadFile(wsPath); err == nil {
		var wsList []model.Workspace
		if err := json.Unmarshal(data, &wsList); err != nil {
			return err
		}
		for i := range wsList {
			s.workspaces[wsList[i].Handle] = &wsList[i]
		}
	}

	// Load entries per workspace
	entriesDir := filepath.Join(s.dataDir, "entries")
	if entries, err := os.ReadDir(entriesDir); err == nil {
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			wsHandle := strings.TrimSuffix(e.Name(), ".json")
			data, err := os.ReadFile(filepath.Join(entriesDir, e.Name()))
			if err != nil {
				continue
			}
			var entryList []model.Entry
			if err := json.Unmarshal(data, &entryList); err != nil {
				continue
			}
			s.entries[wsHandle] = make(map[string]*model.Entry)
			for i := range entryList {
				s.entries[wsHandle][entryList[i].Handle] = &entryList[i]
			}
		}
	}

	// Load audit logs
	auditPath := filepath.Join(s.dataDir, "audit.json")
	if data, err := os.ReadFile(auditPath); err == nil {
		var auditMap map[string][]model.AuditLog
		if err := json.Unmarshal(data, &auditMap); err == nil {
			s.auditLogs = auditMap
			for _, logs := range auditMap {
				for _, log := range logs {
					if log.ID >= s.nextAuditID {
						s.nextAuditID = log.ID + 1
					}
				}
			}
		}
	}

	return nil
}

// saveWorkspaces persists workspaces to disk.
func (s *Store) saveWorkspaces() error {
	wsList := make([]model.Workspace, 0, len(s.workspaces))
	for _, ws := range s.workspaces {
		wsList = append(wsList, *ws)
	}
	data, err := json.MarshalIndent(wsList, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dataDir, "workspaces.json"), data, 0644)
}

// saveEntries persists entries for a workspace to disk.
func (s *Store) saveEntries(wsHandle string) error {
	entries, ok := s.entries[wsHandle]
	if !ok {
		return nil
	}
	entriesDir := filepath.Join(s.dataDir, "entries")
	os.MkdirAll(entriesDir, 0755)

	entryList := make([]model.Entry, 0, len(entries))
	for _, e := range entries {
		entryList = append(entryList, *e)
	}
	data, err := json.MarshalIndent(entryList, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(entriesDir, wsHandle+".json"), data, 0644)
}

// saveAuditLogs persists audit logs to disk.
func (s *Store) saveAuditLogs() error {
	data, err := json.MarshalIndent(s.auditLogs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dataDir, "audit.json"), data, 0644)
}

// --- Workspace operations ---

// CreateWorkspace creates a new workspace.
func (s *Store) CreateWorkspace(name string) (*model.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := &model.Workspace{
		Handle:    model.NewWorkspaceHandle(),
		Name:      name,
		Plan:      "free",
		CreatedAt: time.Now(),
	}
	s.workspaces[ws.Handle] = ws
	s.entries[ws.Handle] = make(map[string]*model.Entry)

	if err := s.saveWorkspaces(); err != nil {
		return nil, err
	}
	return ws, nil
}

// GetWorkspace retrieves a workspace by handle.
func (s *Store) GetWorkspace(handle string) (*model.Workspace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ws, ok := s.workspaces[handle]
	return ws, ok
}

// ListWorkspaces returns all workspaces.
func (s *Store) ListWorkspaces() []*model.Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*model.Workspace, 0, len(s.workspaces))
	for _, ws := range s.workspaces {
		result = append(result, ws)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

// --- Entry operations ---

// PutEntry creates or updates an entry by key within a workspace.
// If an entry with the same key already exists, it updates the value.
func (s *Store) PutEntry(wsHandle, key, value, namespace string, ttl *time.Duration) (*model.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, ok := s.entries[wsHandle]
	if !ok {
		return nil, fmt.Errorf("workspace not found")
	}

	// Check if entry with this key already exists
	for _, e := range entries {
		if e.Key == key && e.Namespace == namespace {
			e.Value = value
			e.UpdatedAt = time.Now()
			if ttl != nil {
				exp := time.Now().Add(*ttl)
				e.ExpiresAt = &exp
			} else {
				e.ExpiresAt = nil
			}
			if err := s.saveEntries(wsHandle); err != nil {
				return nil, err
			}
			return e, nil
		}
	}

	// Create new entry
	entry := &model.Entry{
		Handle:    model.NewStashHandle(),
		Key:       key,
		Value:     value,
		Namespace: namespace,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if ttl != nil {
		exp := time.Now().Add(*ttl)
		entry.ExpiresAt = &exp
	}
	entries[entry.Handle] = entry

	if err := s.saveEntries(wsHandle); err != nil {
		return nil, err
	}
	return entry, nil
}

// GetEntry retrieves an entry by handle.
func (s *Store) GetEntry(wsHandle, handle string) (*model.Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, ok := s.entries[wsHandle]
	if !ok {
		return nil, false
	}
	entry, ok := entries[handle]
	if !ok {
		return nil, false
	}
	if entry.IsExpired() {
		return nil, false
	}
	return entry, true
}

// GetEntryByKey retrieves an entry by key (and optional namespace).
func (s *Store) GetEntryByKey(wsHandle, key, namespace string) (*model.Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, ok := s.entries[wsHandle]
	if !ok {
		return nil, false
	}
	for _, e := range entries {
		if e.Key == key && e.Namespace == namespace {
			if e.IsExpired() {
				return nil, false
			}
			return e, true
		}
	}
	return nil, false
}

// ListEntries returns all entries in a workspace, optionally filtered by namespace.
func (s *Store) ListEntries(wsHandle, namespace string) []*model.Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, ok := s.entries[wsHandle]
	if !ok {
		return nil
	}

	var result []*model.Entry
	for _, e := range entries {
		if e.IsExpired() {
			continue
		}
		if namespace != "" && e.Namespace != namespace {
			continue
		}
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

// DeleteEntry removes an entry by handle.
func (s *Store) DeleteEntry(wsHandle, handle string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, ok := s.entries[wsHandle]
	if !ok {
		return fmt.Errorf("workspace not found")
	}
	if _, ok := entries[handle]; !ok {
		return fmt.Errorf("entry not found")
	}
	delete(entries, handle)

	return s.saveEntries(wsHandle)
}

// DeleteEntryByKey removes an entry by key (and optional namespace).
func (s *Store) DeleteEntryByKey(wsHandle, key, namespace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, ok := s.entries[wsHandle]
	if !ok {
		return fmt.Errorf("workspace not found")
	}

	var foundHandle string
	for h, e := range entries {
		if e.Key == key && e.Namespace == namespace {
			foundHandle = h
			break
		}
	}
	if foundHandle == "" {
		return fmt.Errorf("entry not found")
	}
	delete(entries, foundHandle)

	return s.saveEntries(wsHandle)
}

// CountEntries returns the number of non-expired entries in a workspace.
func (s *Store) CountEntries(wsHandle string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, ok := s.entries[wsHandle]
	if !ok {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsExpired() {
			count++
		}
	}
	return count
}

// --- Audit log operations ---

// AddAuditLog adds an audit log entry.
func (s *Store) AddAuditLog(wsHandle, action, detail, actor string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log := model.AuditLog{
		ID:        s.nextAuditID,
		Workspace: wsHandle,
		Action:    action,
		Detail:    detail,
		Actor:     actor,
		Timestamp: time.Now(),
	}
	s.nextAuditID++

	s.auditLogs[wsHandle] = append(s.auditLogs[wsHandle], log)
	// Keep last 100 logs per workspace
	if len(s.auditLogs[wsHandle]) > 100 {
		s.auditLogs[wsHandle] = s.auditLogs[wsHandle][len(s.auditLogs[wsHandle])-100:]
	}
	s.saveAuditLogs()
}

// ListAuditLogs returns audit logs for a workspace.
func (s *Store) ListAuditLogs(wsHandle string) []model.AuditLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.auditLogs[wsHandle]
}
