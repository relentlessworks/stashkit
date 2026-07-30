package api

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	ctxWorkspace contextKey = "workspace"
	ctxEmail     contextKey = "email"
)

// AuthMiddleware validates the bearer token and loads the workspace context.
func (s *Server) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(w, r, http.StatusUnauthorized,
				"missing auth token",
				"call POST /auth/request with email to get an OTP, then POST /auth/verify to get a bearer token")
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, r, http.StatusUnauthorized,
				"invalid auth header format",
				"use 'Authorization: Bearer <token>' header")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		email, err := s.authManager.ValidateToken(token)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized,
				"invalid or expired token",
				"call POST /auth/request with email to get a new OTP, then POST /auth/verify to get a new bearer token")
			return
		}

		// Get or create default workspace for this user
		wsHandle := r.URL.Query().Get("ws")
		if wsHandle == "" {
			// Use the first workspace or create a default one
			workspaces := s.store.ListWorkspaces()
			if len(workspaces) > 0 {
				wsHandle = workspaces[0].Handle
			} else {
				ws, err := s.store.CreateWorkspace("default")
				if err != nil {
					writeError(w, r, http.StatusInternalServerError, "failed to create workspace", "try again")
					return
				}
				wsHandle = ws.Handle
			}
		} else {
			// Verify workspace exists
			if _, ok := s.store.GetWorkspace(wsHandle); !ok {
				writeError(w, r, http.StatusNotFound,
					"workspace not found",
					"call GET /workspaces to list available workspaces, or POST /workspaces to create one")
				return
			}
		}

		ctx := context.WithValue(r.Context(), ctxWorkspace, wsHandle)
		ctx = context.WithValue(ctx, ctxEmail, email)
		next(w, r.WithContext(ctx))
	}
}

// getWorkspace extracts the workspace handle from the request context.
func getWorkspace(r *http.Request) string {
	if v, ok := r.Context().Value(ctxWorkspace).(string); ok {
		return v
	}
	return ""
}

// getEmail extracts the email from the request context.
func getEmail(r *http.Request) string {
	if v, ok := r.Context().Value(ctxEmail).(string); ok {
		return v
	}
	return ""
}
