package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/relentlessworks/stashkit/internal/auth"
	"github.com/relentlessworks/stashkit/internal/store"
)

func setupTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "stashkit-api-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}

	am := auth.NewManager("test-secret-32-bytes-long-aaaaaa", auth.SMTPConfig{})
	server := NewServer(st, am)

	// Create a workspace
	ws, _ := st.CreateWorkspace("test-workspace")

	return server, ws.Handle
}

func getTestToken(t *testing.T, server *Server, email string) string {
	t.Helper()
	_, err := server.authManager.GenerateOTP(email)
	if err != nil {
		t.Fatal(err)
	}
	token, err := server.authManager.GenerateToken(email)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// doRequest creates a request, sends it through the router, and returns the response recorder.
func doRequest(server *Server, method, path, token string, form url.Values) *httptest.ResponseRecorder {
	var body string
	if form != nil {
		body = form.Encode()
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	server.Routes().ServeHTTP(rr, req)
	return rr
}

func TestHealth(t *testing.T) {
	server, _ := setupTestServer(t)
	rr := doRequest(server, "GET", "/health", "", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "stashkit is running") {
		t.Errorf("expected health message, got: %s", rr.Body.String())
	}
}

func TestHelp(t *testing.T) {
	server, _ := setupTestServer(t)
	rr := doRequest(server, "GET", "/help", "", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "stashkit") {
		t.Error("help should mention stashkit")
	}
	if !strings.Contains(body, "AUTH") {
		t.Error("help should include auth section")
	}
	if !strings.Contains(body, "ENTRIES") {
		t.Error("help should include entries section")
	}
}

func TestAuthRequest(t *testing.T) {
	server, _ := setupTestServer(t)
	form := url.Values{"email": {"test@example.com"}}
	rr := doRequest(server, "POST", "/auth/request", "", form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "OTP") {
		t.Errorf("expected OTP message, got: %s", rr.Body.String())
	}
}

func TestAuthRequestMissingEmail(t *testing.T) {
	server, _ := setupTestServer(t)
	rr := doRequest(server, "POST", "/auth/request", "", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "missing email") {
		t.Errorf("expected missing email error, got: %s", rr.Body.String())
	}
}

func TestAuthVerify(t *testing.T) {
	server, _ := setupTestServer(t)
	email := "test@example.com"

	// Request OTP
	code, _ := server.authManager.GenerateOTP(email)

	// Verify OTP
	form := url.Values{"email": {email}, "code": {code}}
	rr := doRequest(server, "POST", "/auth/verify", "", form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "token=") {
		t.Errorf("expected token in response, got: %s", rr.Body.String())
	}
}

func TestAuthVerifyInvalidCode(t *testing.T) {
	server, _ := setupTestServer(t)
	form := url.Values{"email": {"test@example.com"}, "code": {"000000"}}
	rr := doRequest(server, "POST", "/auth/verify", "", form)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestNoAuthToken(t *testing.T) {
	server, _ := setupTestServer(t)
	rr := doRequest(server, "GET", "/entries", "", nil)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "missing auth token") {
		t.Errorf("expected missing auth token error, got: %s", rr.Body.String())
	}
}

func TestPutAndGetEntry(t *testing.T) {
	server, wsHandle := setupTestServer(t)
	token := getTestToken(t, server, "test@example.com")

	// Put entry
	form := url.Values{"key": {"testkey"}, "value": {"testvalue"}}
	rr := doRequest(server, "POST", "/entries?ws="+wsHandle, token, form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "key=testkey") {
		t.Errorf("expected key in response, got: %s", body)
	}
	if !strings.Contains(body, "value=testvalue") {
		t.Errorf("expected value in response, got: %s", body)
	}

	// Get by key
	rr2 := doRequest(server, "GET", "/lookup?key=testkey&ws="+wsHandle, token, nil)

	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "value=testvalue") {
		t.Errorf("expected value in lookup response, got: %s", rr2.Body.String())
	}
}

func TestPutEntryMissingKey(t *testing.T) {
	server, wsHandle := setupTestServer(t)
	token := getTestToken(t, server, "test@example.com")

	form := url.Values{"value": {"testvalue"}}
	rr := doRequest(server, "POST", "/entries?ws="+wsHandle, token, form)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestListEntries(t *testing.T) {
	server, wsHandle := setupTestServer(t)
	token := getTestToken(t, server, "test@example.com")

	// Add entries
	for i := 0; i < 3; i++ {
		form := url.Values{"key": {"key" + string(rune('0'+i))}, "value": {"val" + string(rune('0'+i))}}
		doRequest(server, "POST", "/entries?ws="+wsHandle, token, form)
	}

	// List
	rr := doRequest(server, "GET", "/entries?ws="+wsHandle, token, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 entries, got %d: %s", len(lines), rr.Body.String())
	}
}

func TestDeleteEntry(t *testing.T) {
	server, wsHandle := setupTestServer(t)
	token := getTestToken(t, server, "test@example.com")

	// Put entry
	form := url.Values{"key": {"delkey"}, "value": {"delvalue"}}
	rr := doRequest(server, "POST", "/entries?ws="+wsHandle, token, form)

	// Extract handle from response
	body := rr.Body.String()
	handleStart := strings.Index(body, "handle=") + 7
	handleEnd := strings.Index(body[handleStart:], " ")
	handle := body[handleStart : handleStart+handleEnd]

	// Delete by handle
	rr2 := doRequest(server, "DELETE", "/entries/"+handle+"?ws="+wsHandle, token, nil)

	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "deleted") {
		t.Errorf("expected deleted message, got: %s", rr2.Body.String())
	}

	// Verify it's gone
	rr3 := doRequest(server, "GET", "/entries/"+handle+"?ws="+wsHandle, token, nil)
	if rr3.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", rr3.Code)
	}
}

func TestDeleteEntryByKey(t *testing.T) {
	server, wsHandle := setupTestServer(t)
	token := getTestToken(t, server, "test@example.com")

	// Put entry
	form := url.Values{"key": {"delkey"}, "value": {"delvalue"}}
	doRequest(server, "POST", "/entries?ws="+wsHandle, token, form)

	// Delete by key
	rr := doRequest(server, "DELETE", "/lookup?key=delkey&ws="+wsHandle, token, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Verify gone
	rr2 := doRequest(server, "GET", "/lookup?key=delkey&ws="+wsHandle, token, nil)
	if rr2.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr2.Code)
	}
}

func TestWorkspaces(t *testing.T) {
	server, _ := setupTestServer(t)
	token := getTestToken(t, server, "test@example.com")

	// List workspaces
	rr := doRequest(server, "GET", "/workspaces", token, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "test-workspace") {
		t.Errorf("expected test-workspace in list, got: %s", rr.Body.String())
	}

	// Create new workspace
	form := url.Values{"name": {"new-ws"}}
	rr2 := doRequest(server, "POST", "/workspaces", token, form)

	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "name=new-ws") {
		t.Errorf("expected new-ws in response, got: %s", rr2.Body.String())
	}
}

func TestJSONFormat(t *testing.T) {
	server, wsHandle := setupTestServer(t)
	token := getTestToken(t, server, "test@example.com")

	// Put entry
	form := url.Values{"key": {"jsonkey"}, "value": {"jsonvalue"}}
	doRequest(server, "POST", "/entries?ws="+wsHandle, token, form)

	// Get with JSON format
	rr := doRequest(server, "GET", "/lookup?key=jsonkey&ws="+wsHandle+"&format=json", token, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Errorf("expected valid JSON, got error: %v, body: %s", err, rr.Body.String())
	}
	if result["key"] != "jsonkey" {
		t.Errorf("expected key 'jsonkey', got: %v", result["key"])
	}
	if result["value"] != "jsonvalue" {
		t.Errorf("expected value 'jsonvalue', got: %v", result["value"])
	}
}

func TestAuditLog(t *testing.T) {
	server, wsHandle := setupTestServer(t)
	token := getTestToken(t, server, "test@example.com")

	// Do some operations
	form := url.Values{"key": {"auditkey"}, "value": {"auditvalue"}}
	doRequest(server, "POST", "/entries?ws="+wsHandle, token, form)

	// Get audit log
	rr := doRequest(server, "GET", "/audit?ws="+wsHandle, token, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "entry.put") {
		t.Errorf("expected entry.put in audit log, got: %s", rr.Body.String())
	}
}

func TestMCP(t *testing.T) {
	server, _ := setupTestServer(t)
	rr := doRequest(server, "GET", "/mcp", "", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "MCP") {
		t.Errorf("expected MCP in response, got: %s", rr.Body.String())
	}
}

func TestNamespaceSupport(t *testing.T) {
	server, wsHandle := setupTestServer(t)
	token := getTestToken(t, server, "test@example.com")

	// Put entry in namespace
	form := url.Values{"key": {"nskey"}, "value": {"nsvalue"}, "namespace": {"myns"}}
	rr := doRequest(server, "POST", "/entries?ws="+wsHandle, token, form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "namespace=myns") {
		t.Errorf("expected namespace in response, got: %s", rr.Body.String())
	}

	// Lookup with namespace
	rr2 := doRequest(server, "GET", "/lookup?key=nskey&namespace=myns&ws="+wsHandle, token, nil)

	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "value=nsvalue") {
		t.Errorf("expected value in response, got: %s", rr2.Body.String())
	}

	// Lookup without namespace should fail
	rr3 := doRequest(server, "GET", "/lookup?key=nskey&ws="+wsHandle, token, nil)
	if rr3.Code != http.StatusNotFound {
		t.Errorf("expected 404 without namespace, got %d", rr3.Code)
	}
}
