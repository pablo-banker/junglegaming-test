package auth

import "slices"

// Principal represents an authenticated service identity.
type Principal struct {
	Subject    string
	ClientID   string
	ProviderID string
	Roles      []string
}

// HasRole reports whether the principal has the requested role.
func (p Principal) HasRole(role string) bool {
	return slices.Contains(p.Roles, role)
}

// IsProvider reports whether the principal represents a game provider.
func (p Principal) IsProvider() bool {
	return p.ProviderID != "" && p.HasRole("provider")
}

// IsInternal reports whether the principal represents an internal service.
func (p Principal) IsInternal() bool {
	return p.HasRole("internal")
}
