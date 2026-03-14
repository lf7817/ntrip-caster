// Package auth provides HTTP Basic Auth parsing and NTRIP authentication helpers.
package auth

import (
	"encoding/base64"
	"strings"
)

// ParseBasicAuth extracts username and password from an HTTP Authorization
// header value (e.g. "Basic dXNlcjpwYXNz"). Returns ("","",false) on failure.
func ParseBasicAuth(authHeader string) (username, password string, ok bool) {
	const prefix = "Basic "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(authHeader[len(prefix):])
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
