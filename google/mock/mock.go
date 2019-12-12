package mock

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func OauthHandler(t *testing.T, oauthRedirect string) func(w http.ResponseWriter, r *http.Request) {
	u, err := url.Parse(oauthRedirect)
	if err != nil {
		t.Error(err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		t.Log("mock google handler handling", r.URL.String())
		if r.URL.Path == "/oauth2/auth" {
			next := *u
			v := next.Query()
			v.Set("code", "acode")
			next.RawQuery = v.Encode()
			http.Redirect(w, r, next.String(), http.StatusFound)
			return
		}

		if r.URL.Path == "/oauth2/token" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(oauth2.Token{
				AccessToken:  "test_access_token",
				TokenType:    "bearer",
				RefreshToken: "test_refresh_token",
				Expiry:       time.Now().Add(time.Hour),
			}); err != nil {
				t.Error(err)
			}
			return
		}

		http.NotFound(w, r)
	}
}
