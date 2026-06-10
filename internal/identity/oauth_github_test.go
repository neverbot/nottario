package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCheckOrgMembership covers the three outcomes the OAuth callback
// cares about: active member, pending member (reject), and not a
// member at all. We don't drive the full OAuth dance — only the
// helper that gates UpsertFromGithub.
func TestCheckOrgMembership(t *testing.T) {
	const org = "acme"

	cases := []struct {
		name   string
		status int
		body   string
		want   bool
		errIs  string
	}{
		{name: "active member", status: 200, body: `{"state":"active"}`, want: true},
		{name: "pending member", status: 200, body: `{"state":"pending"}`, want: false},
		{name: "not a member", status: 404, body: `{"message":"Not Found"}`, want: false},
		{name: "server error surfaces", status: 500, body: `boom`, errIs: "membership status 500"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/user/memberships/orgs/"+org {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer ts.Close()
			prev := githubAPIBase
			githubAPIBase = ts.URL
			defer func() { githubAPIBase = prev }()

			got, err := checkOrgMembership(context.Background(), http.DefaultClient, org)
			if tc.errIs != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errIs) {
					t.Fatalf("want err containing %q, got %v", tc.errIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}

// TestNewOAuth2ConfigScopes confirms the read:org scope is only added
// when the gate is on. The narrower scope keeps the consent screen
// minimal for instances without an org restriction.
func TestNewOAuth2ConfigScopes(t *testing.T) {
	cfg := OAuthConfig{ClientID: "x", ClientSecret: "y", PublicURL: "https://h"}
	if hasScope(newOAuth2Config(cfg).Scopes, "read:org") {
		t.Error("read:org should NOT be requested when RequiredOrg is empty")
	}
	cfg.RequiredOrg = "acme"
	if !hasScope(newOAuth2Config(cfg).Scopes, "read:org") {
		t.Error("read:org MUST be requested when RequiredOrg is set")
	}
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}
