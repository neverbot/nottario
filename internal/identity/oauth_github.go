package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

// OAuthConfig captures the OAuth-related runtime configuration. It
// keeps the identity package independent from internal/config to
// avoid an import cycle once REST handlers compose them.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	PublicURL    string
	SessionKey   []byte
	CookieSecure bool
	// RequiredOrg, when non-empty, restricts logins to active members
	// of this GitHub organisation. Empty disables the gate.
	RequiredOrg string
}

// ErrOrgRequired is returned by HandleGithubCallback when the
// authenticated GitHub user is not an active member of the
// configured org. The HTTP layer turns this into a redirect to
// /login?error=org_required so the user sees a flash, not a JSON
// error blob.
var ErrOrgRequired = errors.New("org membership required")

// stateCookieName carries the random CSRF state across the OAuth
// redirect.
const stateCookieName = "nottario_oauth_state"

const githubAuthURL = "https://github.com/login/oauth/authorize"
const githubTokenURL = "https://github.com/login/oauth/access_token"

// githubAPIBase is overridable in tests. The user/emails endpoints are
// formed from this base inside fetchGithubUser.
var githubAPIBase = "https://api.github.com"

// oauthEndpoint returns the GitHub OAuth2 endpoint; tests can
// substitute a different base if needed.
func oauthEndpoint() oauth2.Endpoint {
	return oauth2.Endpoint{
		AuthURL:  githubAuthURL,
		TokenURL: githubTokenURL,
	}
}

func newOAuth2Config(c OAuthConfig) *oauth2.Config {
	// read:org is only requested when the gate is on; otherwise we keep
	// the consent screen as narrow as possible.
	scopes := []string{"read:user", "user:email"}
	if c.RequiredOrg != "" {
		scopes = append(scopes, "read:org")
	}
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint:     oauthEndpoint(),
		RedirectURL:  strings.TrimRight(c.PublicURL, "/") + "/auth/github/callback",
		Scopes:       scopes,
	}
}

// BeginGithubAuth writes a signed state cookie and redirects to GitHub.
func BeginGithubAuth(w http.ResponseWriter, r *http.Request, c OAuthConfig) {
	state := randomHex(24)
	mac := hmac.New(sha256.New, c.SessionKey)
	mac.Write([]byte(state))
	cookieValue := state + "." + hex.EncodeToString(mac.Sum(nil))

	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    cookieValue,
		Path:     "/auth/github",
		HttpOnly: true,
		Secure:   c.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((10 * time.Minute).Seconds()),
	})

	url := newOAuth2Config(c).AuthCodeURL(state, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusFound)
}

// HandleGithubCallback completes the OAuth exchange, upserts the
// user, opens a session and sets the session cookie. On success it
// returns the new Session.
func HandleGithubCallback(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, c OAuthConfig) (*Session, error) {
	stateQuery := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if stateQuery == "" || code == "" {
		return nil, errors.New("missing state or code")
	}
	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		return nil, errors.New("missing state cookie")
	}
	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 || parts[0] != stateQuery {
		return nil, errors.New("state mismatch")
	}
	mac := hmac.New(sha256.New, c.SessionKey)
	mac.Write([]byte(parts[0]))
	want := mac.Sum(nil)
	got, err := hex.DecodeString(parts[1])
	if err != nil || !hmac.Equal(want, got) {
		return nil, errors.New("state signature mismatch")
	}
	// Clear the state cookie eagerly.
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Value: "", Path: "/auth/github", MaxAge: -1})

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	tok, err := newOAuth2Config(c).Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oauth exchange: %w", err)
	}

	client := newOAuth2Config(c).Client(ctx, tok)
	profile, err := fetchGithubUser(ctx, client)
	if err != nil {
		return nil, err
	}

	if c.RequiredOrg != "" {
		ok, err := checkOrgMembership(ctx, client, c.RequiredOrg)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrOrgRequired
		}
	}

	user, _, err := UpsertFromGithub(ctx, pool, profile.ID, profile.Login, displayNameOf(profile), profile.AvatarURL)
	if err != nil {
		return nil, err
	}
	_ = TouchUserSeen(ctx, pool, user.ID)

	sess, err := NewSession(ctx, pool, user.ID, r.UserAgent(), clientIP(r))
	if err != nil {
		return nil, err
	}

	SetSessionCookie(w, EncodeCookie(sess.ID, c.SessionKey), c.CookieSecure)
	return sess, nil
}

type githubProfile struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email"`
}

func fetchGithubUser(ctx context.Context, client *http.Client) (*githubProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIBase+"/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github /user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github /user status %d: %s", resp.StatusCode, string(body))
	}
	var p githubProfile
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode github user: %w", err)
	}
	return &p, nil
}

// checkOrgMembership returns true when the authenticated user is an
// active member of org. Uses GET /user/memberships/orgs/{org}, which
// works on the caller's own membership and only requires the
// `read:org` scope (which we ask for when RequiredOrg is set).
// A 404 means the user is not a member; any state other than
// "active" (e.g. "pending") also rejects. Other statuses surface as
// an error so the operator can see what's wrong.
func checkOrgMembership(ctx context.Context, client *http.Client, org string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIBase+"/user/memberships/orgs/"+org, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("github membership: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("github membership status %d: %s", resp.StatusCode, string(body))
	}
	var m struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return false, fmt.Errorf("decode github membership: %w", err)
	}
	return m.State == "active", nil
}

func displayNameOf(p *githubProfile) string {
	if strings.TrimSpace(p.Name) != "" {
		return p.Name
	}
	return p.Login
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if r.RemoteAddr == "" {
		return ""
	}
	if i := strings.LastIndex(r.RemoteAddr, ":"); i >= 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

func randomHex(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
