package janus

import (
	"context"
	"strings"

	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/hellofresh/janus/errors"
	"github.com/hellofresh/janus/oauth"
	"github.com/hellofresh/janus/request"
	"github.com/hellofresh/janus/session"
)

// Enums for keys to be stored in a session context - this is how gorilla expects
// these to be implemented and is lifted pretty much from docs
var (
	SessionData     = request.ContextKey("session_data")
	AuthHeaderValue = request.ContextKey("auth_header")
)

// Oauth2KeyExistsMiddleware checks the integrity of the provided OAuth headers
type Oauth2KeyExistsMiddleware struct {
	manager *oauth.Manager
}

func NewOauth2KeyExistsMiddleware(manager *oauth.Manager) *Oauth2KeyExistsMiddleware {
	return &Oauth2KeyExistsMiddleware{manager}
}

// Handler is the middleware method.
func (m *Oauth2KeyExistsMiddleware) Handler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debug("Starting Oauth2KeyExists middleware")
		logger := log.WithFields(log.Fields{
			"path":   r.RequestURI,
			"origin": r.RemoteAddr,
		})

		// We're using OAuth, start checking for access keys
		authHeaderValue := r.Header.Get("Authorization")
		parts := strings.Split(authHeaderValue, " ")
		if len(parts) < 2 {
			logger.Info("Attempted access with malformed header, no auth header found.")
			panic(errors.New(http.StatusBadRequest, "authorization field missing"))
		}

		if strings.ToLower(parts[0]) != "bearer" {
			logger.Info("Bearer token malformed")
			panic(errors.New(http.StatusBadRequest, "bearer token malformed"))
		}

		accessToken := parts[1]
		thisSessionState, keyExists := m.CheckSessionAndIdentityForValidKey(accessToken)

		//TODO find a way to check this thisSessionState.OAuthServerID != m.Spec.OAuthServerID
		if !keyExists {
			log.WithFields(log.Fields{
				"path":   r.RequestURI,
				"origin": r.RemoteAddr,
				"key":    accessToken,
			}).Info("Attempted access with non-existent key.")

			panic(errors.New(http.StatusUnauthorized, "key not authorised"))
		}

		ctx := context.WithValue(r.Context(), SessionData, thisSessionState)
		ctx = context.WithValue(ctx, AuthHeaderValue, accessToken)

		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CheckSessionAndIdentityForValidKey ensures we have the valid key in the session store
func (m *Oauth2KeyExistsMiddleware) CheckSessionAndIdentityForValidKey(key string) (session.SessionState, bool) {
	var thisSession session.SessionState

	// Checks if the key is present on the cache and if it didn't expire yet
	log.Debug("Querying keystore")
	if !m.manager.KeyExists(key) {
		log.Debug("Key not found in keystore")
		return thisSession, false
	}

	// 2. If not there, get it from the AuthorizationHandler
	return m.manager.IsKeyAuthorised(key)
}
