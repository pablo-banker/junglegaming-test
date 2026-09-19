package keycloak

// RealmAccess contains roles assigned by the Keycloak realm.
type RealmAccess struct {
	Roles []string `json:"roles"`
}

// Claims contains identity information extracted from a Keycloak access token.
type Claims struct {
	Subject         string      `json:"sub"`
	AuthorizedParty string      `json:"azp"`
	ProviderID      string      `json:"provider_id"`
	RealmAccess     RealmAccess `json:"realm_access"`
}
