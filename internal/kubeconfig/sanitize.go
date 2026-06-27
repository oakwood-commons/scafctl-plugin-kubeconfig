package kubeconfig

import (
	"regexp"
	"strings"
)

// sanitizeInvalidChars matches any run of characters that are not allowed in a
// kubeconfig cluster/context/user name. Valid characters are lowercase
// alphanumerics plus '-' and '.', mirroring DNS subdomain conventions used by
// kubeconfig tooling.
var sanitizeInvalidChars = regexp.MustCompile(`[^a-z0-9.-]+`)

// sanitizeName normalizes an arbitrary string into a stable, kubeconfig-safe
// name. It lowercases, replaces runs of invalid characters with a single '-',
// and trims leading/trailing separators. An empty or fully-invalid input
// yields the supplied fallback.
//
// This helper is intentionally small and dependency-free so the Phase 4
// OpenShift handler can reuse the same normalization.
func sanitizeName(raw, fallback string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = sanitizeInvalidChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-.")
	if s == "" {
		return fallback
	}
	return s
}
