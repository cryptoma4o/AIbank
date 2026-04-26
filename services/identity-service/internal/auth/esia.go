package auth

import "errors"

// ErrESIANotConfigured is returned when ЕСИА is not configured for the tenant.
var ErrESIANotConfigured = errors.New("esia: not configured for this tenant")

// ESIAClient is a Pre-MVP stub for ЕСИА OAuth2 integration.
// Production: OAuth2 PKCE flow with Gosuslugi, signed requests via GOST 34.10-2012.
type ESIAClient struct {
	clientID    string
	redirectURI string
}

func NewESIAClient(clientID, redirectURI string) *ESIAClient {
	return &ESIAClient{clientID: clientID, redirectURI: redirectURI}
}

// AuthURL returns the ЕСИА authorization URL.
func (c *ESIAClient) AuthURL(state string) string {
	return "https://esia.gosuslugi.ru/aas/oauth2/ac?client_id=" + c.clientID + "&state=" + state
}

// ExchangeCode exchanges an authorization code for user info.
// Pre-MVP stub — returns ErrESIANotConfigured always.
func (c *ESIAClient) ExchangeCode(_ string) (inn, phone, name string, err error) {
	return "", "", "", ErrESIANotConfigured
}
