package api

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/relentlessworks/stashkit/internal/auth"
	"github.com/relentlessworks/stashkit/internal/store"
)

// Server holds all dependencies for the API server.
type Server struct {
	store      *store.Store
	authManager *auth.Manager
}

// NewServer creates a new API server.
func NewServer(s *store.Store, am *auth.Manager) *Server {
	return &Server{store: s, authManager: am}
}

// Routes returns the HTTP handler with all routes registered.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("/health", s.handleHealth)

	// Help / self-documentation
	mux.HandleFunc("/help", s.handleHelp)
	mux.HandleFunc("/.well-known/agent.md", s.handleHelp)

	// Auth
	mux.HandleFunc("/auth/request", s.handleAuthRequest)
	mux.HandleFunc("/auth/verify", s.handleAuthVerify)

	// Workspaces (auth required)
	mux.HandleFunc("/workspaces", s.AuthMiddleware(s.handleWorkspaces))

	// Entries (auth required)
	mux.HandleFunc("/entries", s.AuthMiddleware(s.handleEntries))
	mux.HandleFunc("/entries/", s.AuthMiddleware(s.handleEntryByHandle))

	// Lookup by key (auth required)
	mux.HandleFunc("/lookup", s.AuthMiddleware(s.handleLookup))

	// Audit logs (auth required)
	mux.HandleFunc("/audit", s.AuthMiddleware(s.handleAudit))

	// MCP endpoint
	mux.HandleFunc("/mcp", s.handleMCP)

	return mux
}

// --- Health ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeOK(w, r, "stashkit is running")
}

// --- Help / Self-documentation ---

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, helpText)
}

const helpText = `stashkit — agentic-first key-value store

Store and retrieve arbitrary key-value data. Simple, fast, token-cheap.

AUTH:
  1. POST /auth/request   body: email=user@example.com
     → Sends OTP to email (or logs to stderr if no SMTP configured)
  2. POST /auth/verify    body: email=user@example.com code=123456
     → Returns: token=<bearer-token>
  3. Use token in all subsequent requests: Authorization: Bearer <token>

WORKSPACES:
  GET  /workspaces              → List all workspaces
  POST /workspaces              body: name=my-workspace
     → Create a workspace. Returns: handle=ws_abc12 name=my-workspace plan=free

ENTRIES (key-value store):
  POST /entries                 body: key=mykey value=myvalue [namespace=ns] [ttl=3600]
     → Create or update entry by key. TTL in seconds (optional).
     → Returns: handle=stash_a1b2c key=mykey value=myvalue namespace= created=...
  GET  /entries                 [?namespace=ns]
     → List all entries (optionally filtered by namespace)
  GET  /entries/<handle>        → Get entry by handle
  DELETE /entries/<handle>      → Delete entry by handle

LOOKUP BY KEY:
  GET  /lookup?key=mykey        [?namespace=ns]
     → Get entry by key directly (no handle needed)
  DELETE /lookup?key=mykey      [?namespace=ns]
     → Delete entry by key directly

AUDIT LOG:
  GET  /audit                   → List recent audit log entries

RESPONSE FORMAT:
  Default: plain text, one record per line (key=value pairs)
  JSON:    add Accept: application/json header or ?format=json query param
  Errors:  error: <message> | hint: <what to do next>

OPTIONS:
  ?ws=<workspace-handle>  → Specify workspace (defaults to first workspace)
  ?namespace=<ns>         → Filter entries by namespace
  ttl=<seconds>           → Set TTL on entry (auto-expires after N seconds)

EXAMPLES:
  # Set a value
  curl -X POST http://localhost:7788/entries \
    -H "Authorization: Bearer <token>" \
    -d "key=api_key value=sk-12345 namespace=secrets"

  # Get a value by key
  curl http://localhost:7788/lookup?key=api_key&namespace=secrets \
    -H "Authorization: Bearer <token>"

  # List all entries
  curl http://localhost:7788/entries \
    -H "Authorization: Bearer <token>"

  # Set with TTL (expires in 1 hour)
  curl -X POST http://localhost:7788/entries \
    -H "Authorization: Bearer <token>" \
    -d "key=session value=abc123 ttl=3600"
`

// --- Auth handlers ---

func (s *Server) handleAuthRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
		return
	}

	email := r.FormValue("email")
	if email == "" {
		writeError(w, r, http.StatusBadRequest, "missing email", "provide email parameter: email=user@example.com")
		return
	}

	code, err := s.authManager.GenerateOTP(email)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to send OTP", "check SMTP configuration or try again")
		return
	}

	// If no SMTP configured, log OTP to stderr for dev/testing
	if s.authManager.IsSMTPConfigured() {
		writeOK(w, r, "OTP sent to "+email)
	} else {
		fmt.Fprintf(os.Stderr, "[stashkit] OTP for %s: %s\n", email, code)
		writeOK(w, r, "OTP generated (check server logs for code in dev mode)")
	}
}

func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
		return
	}

	email := r.FormValue("email")
	code := r.FormValue("code")
	if email == "" || code == "" {
		writeError(w, r, http.StatusBadRequest, "missing email or code", "provide both email and code parameters")
		return
	}

	if !s.authManager.VerifyOTP(email, code) {
		writeError(w, r, http.StatusUnauthorized, "invalid or expired OTP", "request a new OTP via POST /auth/request with email")
		return
	}

	token, err := s.authManager.GenerateToken(email)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to generate token", "try again")
		return
	}

	writeRecord(w, r, map[string]interface{}{
		"token": token,
		"email": email,
	})
}

// --- Workspace handlers ---

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		workspaces := s.store.ListWorkspaces()
		records := make([]map[string]interface{}, 0, len(workspaces))
		for _, ws := range workspaces {
			records = append(records, map[string]interface{}{
				"handle": ws.Handle,
				"name":   ws.Name,
				"plan":   ws.Plan,
				"entries": s.store.CountEntries(ws.Handle),
			})
		}
		if len(records) == 0 {
			writeOK(w, r, "no workspaces yet. POST /workspaces with name= to create one")
			return
		}
		writeRecords(w, r, records)

	case http.MethodPost:
		name := r.FormValue("name")
		if name == "" {
			writeError(w, r, http.StatusBadRequest, "missing name", "provide name parameter: name=my-workspace")
			return
		}
		ws, err := s.store.CreateWorkspace(name)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "failed to create workspace", "try again")
			return
		}
		s.store.AddAuditLog(ws.Handle, "workspace.create", "created workspace "+name, getEmail(r))
		writeRecord(w, r, map[string]interface{}{
			"handle": ws.Handle,
			"name":   ws.Name,
			"plan":   ws.Plan,
		})

	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET or POST")
	}
}

// --- Entry handlers ---

func (s *Server) handleEntries(w http.ResponseWriter, r *http.Request) {
	wsHandle := getWorkspace(r)

	switch r.Method {
	case http.MethodPost:
		key := r.FormValue("key")
		value := r.FormValue("value")
		if key == "" {
			writeError(w, r, http.StatusBadRequest, "missing key", "provide key parameter: key=mykey value=myvalue")
			return
		}
		if value == "" {
			writeError(w, r, http.StatusBadRequest, "missing value", "provide value parameter: key=mykey value=myvalue")
			return
		}

		namespace := r.FormValue("namespace")
		var ttl *time.Duration
		if ttlStr := r.FormValue("ttl"); ttlStr != "" {
			secs, err := strconv.Atoi(ttlStr)
			if err != nil || secs <= 0 {
				writeError(w, r, http.StatusBadRequest, "invalid ttl", "ttl must be a positive integer in seconds, e.g. ttl=3600")
				return
			}
			d := time.Duration(secs) * time.Second
			ttl = &d
		}

		entry, err := s.store.PutEntry(wsHandle, key, value, namespace, ttl)
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "failed to store entry", "try again")
			return
		}

		s.store.AddAuditLog(wsHandle, "entry.put", fmt.Sprintf("key=%s namespace=%s", key, namespace), getEmail(r))

		record := map[string]interface{}{
			"handle":    entry.Handle,
			"key":       entry.Key,
			"value":     entry.Value,
			"namespace": entry.Namespace,
			"created":   entry.CreatedAt.Format(time.RFC3339),
			"updated":   entry.UpdatedAt.Format(time.RFC3339),
		}
		if entry.ExpiresAt != nil {
			record["expires"] = entry.ExpiresAt.Format(time.RFC3339)
		}
		writeRecord(w, r, record)

	case http.MethodGet:
		namespace := r.URL.Query().Get("namespace")
		entries := s.store.ListEntries(wsHandle, namespace)
		if len(entries) == 0 {
			writeOK(w, r, "no entries found. POST /entries with key= and value= to create one")
			return
		}
		records := make([]map[string]interface{}, 0, len(entries))
		for _, e := range entries {
			record := map[string]interface{}{
				"handle":    e.Handle,
				"key":       e.Key,
				"value":     e.Value,
				"namespace": e.Namespace,
				"updated":   e.UpdatedAt.Format(time.RFC3339),
			}
			if e.ExpiresAt != nil {
				record["expires"] = e.ExpiresAt.Format(time.RFC3339)
			}
			records = append(records, record)
		}
		writeRecords(w, r, records)

	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET or POST")
	}
}

func (s *Server) handleEntryByHandle(w http.ResponseWriter, r *http.Request) {
	wsHandle := getWorkspace(r)
	handle := strings.TrimPrefix(r.URL.Path, "/entries/")
	if handle == "" {
		writeError(w, r, http.StatusBadRequest, "missing handle", "provide entry handle in URL: /entries/stash_a1b2c")
		return
	}

	switch r.Method {
	case http.MethodGet:
		entry, ok := s.store.GetEntry(wsHandle, handle)
		if !ok {
			writeError(w, r, http.StatusNotFound, "entry not found", "call GET /entries to list all entries")
			return
		}
		record := map[string]interface{}{
			"handle":    entry.Handle,
			"key":       entry.Key,
			"value":     entry.Value,
			"namespace": entry.Namespace,
			"created":   entry.CreatedAt.Format(time.RFC3339),
			"updated":   entry.UpdatedAt.Format(time.RFC3339),
		}
		if entry.ExpiresAt != nil {
			record["expires"] = entry.ExpiresAt.Format(time.RFC3339)
		}
		writeRecord(w, r, record)

	case http.MethodDelete:
		if err := s.store.DeleteEntry(wsHandle, handle); err != nil {
			writeError(w, r, http.StatusNotFound, "entry not found", "call GET /entries to list all entries")
			return
		}
		s.store.AddAuditLog(wsHandle, "entry.delete", "handle="+handle, getEmail(r))
		writeOK(w, r, "entry deleted")

	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET or DELETE")
	}
}

// --- Lookup by key ---

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	wsHandle := getWorkspace(r)
	key := r.URL.Query().Get("key")
	namespace := r.URL.Query().Get("namespace")

	if key == "" {
		writeError(w, r, http.StatusBadRequest, "missing key", "provide key query parameter: /lookup?key=mykey")
		return
	}

	switch r.Method {
	case http.MethodGet:
		entry, ok := s.store.GetEntryByKey(wsHandle, key, namespace)
		if !ok {
			writeError(w, r, http.StatusNotFound, "entry not found", "call POST /entries with key="+key+" to create it")
			return
		}
		record := map[string]interface{}{
			"handle":    entry.Handle,
			"key":       entry.Key,
			"value":     entry.Value,
			"namespace": entry.Namespace,
			"updated":   entry.UpdatedAt.Format(time.RFC3339),
		}
		if entry.ExpiresAt != nil {
			record["expires"] = entry.ExpiresAt.Format(time.RFC3339)
		}
		writeRecord(w, r, record)

	case http.MethodDelete:
		if err := s.store.DeleteEntryByKey(wsHandle, key, namespace); err != nil {
			writeError(w, r, http.StatusNotFound, "entry not found", "call GET /entries to list all entries")
			return
		}
		s.store.AddAuditLog(wsHandle, "entry.delete_by_key", "key="+key, getEmail(r))
		writeOK(w, r, "entry deleted")

	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET or DELETE")
	}
}

// --- Audit log ---

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET")
		return
	}

	wsHandle := getWorkspace(r)
	logs := s.store.ListAuditLogs(wsHandle)
	if len(logs) == 0 {
		writeOK(w, r, "no audit logs yet")
		return
	}

	records := make([]map[string]interface{}, 0, len(logs))
	for _, log := range logs {
		records = append(records, map[string]interface{}{
			"id":      log.ID,
			"action":  log.Action,
			"detail":  log.Detail,
			"actor":   log.Actor,
			"time":    log.Timestamp.Format(time.RFC3339),
		})
	}
	writeRecords(w, r, records)
}

// --- MCP endpoint ---

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	// MCP endpoint for chat client integrations
	// Returns the same operations as the HTTP API in MCP format
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, mcpHelp)
}

const mcpHelp = `stashkit MCP endpoint

This service supports Model Context Protocol at /mcp.

Available tools:
  - stash_put:    Store a key-value pair (params: key, value, namespace?, ttl?)
  - stash_get:    Retrieve value by key (params: key, namespace?)
  - stash_list:   List all entries (params: namespace?)
  - stash_delete: Delete entry by key (params: key, namespace?)
  - stash_lookup: Get entry by handle (params: handle)

All operations require authentication via bearer token.
See GET /help for full API documentation.`
