package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// discordAPI is the Discord API base.
const discordAPI = "https://discord.com/api"

// oauthClient performs the Discord OAuth2 authorization-code flow with PKCE. It
// uses plain HTTP (no gateway), so it is independent of the bot session.
type oauthClient struct {
	clientID     string
	clientSecret string
	redirectURI  string
	http         *http.Client
}

func newOAuthClient(clientID, clientSecret, redirectURI string) *oauthClient {
	return &oauthClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		http:         &http.Client{Timeout: 15 * time.Second},
	}
}

// authCodeURL builds the authorize URL for the identify+guilds scopes with a
// PKCE challenge derived from verifier.
func (c *oauthClient) authCodeURL(state, verifier string) string {
	challenge := pkceChallenge(verifier)
	q := url.Values{
		"client_id":             {c.clientID},
		"response_type":         {"code"},
		"scope":                 {"identify guilds"},
		"redirect_uri":          {c.redirectURI},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"prompt":                {"none"},
	}
	return "https://discord.com/oauth2/authorize?" + q.Encode()
}

// exchange trades an authorization code for an access token.
func (c *oauthClient) exchange(ctx context.Context, code, verifier string) (string, error) {
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.redirectURI},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordAPI+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("token exchange: status %d: %s", resp.StatusCode, body)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("token exchange: empty access token")
	}
	return out.AccessToken, nil
}

// DiscordUser is the subset of /users/@me we use.
type DiscordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

// UserGuild is the subset of /users/@me/guilds we use.
type UserGuild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

func (c *oauthClient) me(ctx context.Context, accessToken string) (DiscordUser, error) {
	var u DiscordUser
	err := c.get(ctx, accessToken, "/users/@me", &u)
	return u, err
}

func (c *oauthClient) myGuilds(ctx context.Context, accessToken string) ([]UserGuild, error) {
	var g []UserGuild
	err := c.get(ctx, accessToken, "/users/@me/guilds", &g)
	return g, err
}

func (c *oauthClient) get(ctx context.Context, accessToken, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discordAPI+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// pkceChallenge returns the S256 challenge for a verifier.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// randomToken returns n bytes of URL-safe randomness.
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
