package cmd

import (
	"regexp"

	"github.com/rpuneet/mycel/pkg/client"
)

// getClient returns an HTTP client for the daemon server.
func getClient() *client.Client {
	return client.New("")
}

// identifierPattern matches valid identifiers: starts with a letter or underscore,
// followed by letters, numbers, dashes, underscores, or colons (for gateway channels like "slack:eng").
var identifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_:.-]*$`)

// validIdentifier checks if a string is a valid identifier (alphanumeric, dash, underscore).
// Must start with letter or underscore, then letters, numbers, dashes, underscores.
func validIdentifier(s string) bool {
	if s == "" {
		return false
	}
	return identifierPattern.MatchString(s)
}

// isValidRoleName checks if a role name is valid.
func isValidRoleName(name string) bool {
	if name == "" || len(name) > 50 {
		return false
	}
	for _, ch := range name {
		isLower := ch >= 'a' && ch <= 'z'
		isDigit := ch >= '0' && ch <= '9'
		isValid := isLower || isDigit || ch == '-' || ch == '_'
		if !isValid {
			return false
		}
	}
	return true
}
