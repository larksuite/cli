package auth

import (
	"context"
	"net/http"
)

type authenticationBoundaryKey struct{}

// AuthenticationRequest suppresses retry/challenge transports: the caller owns
// the single-send and sanitized response contract for credential-bearing HTTP.
func AuthenticationRequest(req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), authenticationBoundaryKey{}, true))
}
func IsAuthenticationRequest(req *http.Request) bool {
	marked, _ := req.Context().Value(authenticationBoundaryKey{}).(bool)
	return marked
}
