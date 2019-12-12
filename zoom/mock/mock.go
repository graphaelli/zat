package mock

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/graphaelli/zat/zoom"
)

// ApiHandler mocks the zoom API endpoints
func ApiHandler(t *testing.T) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		t.Log("mock zoom handler handling", r.URL.String())

		if r.URL.Path == "/api/v2/users/me/recordings" {
			if err := json.NewEncoder(w).Encode(zoom.ListRecordingsResponse{
				From:          "",
				To:            "",
				PageCount:     0,
				PageSize:      0,
				TotalRecords:  0,
				NextPageToken: "",
				Meetings:      nil,
			}); err != nil {
				t.Error(err)
			}
			return
		}

		http.NotFound(w, r)
	}
}

type OauthResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// OauthHandler mocks the zoom OAuth endpoints
func OauthHandler(t *testing.T, oauthRedirect string) func(w http.ResponseWriter, r *http.Request) {
	u, err := url.Parse(oauthRedirect)
	if err != nil {
		t.Error(err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		t.Log("mock zoom handler handling", r.URL.String())
		if r.URL.Path == "/oauth/authorize" {
			next := *u
			v := next.Query()
			v.Set("code", "acode")
			next.RawQuery = v.Encode()
			http.Redirect(w, r, next.String(), http.StatusFound)
			return
		}

		if r.URL.Path == "/oauth/token" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(OauthResponse{
				AccessToken:  "test_access_token",
				TokenType:    "bearer",
				RefreshToken: "test_refresh_token",
				ExpiresIn:    3600,
				Scope:        "test_scope",
			}); err != nil {
				t.Error(err)
			}
			return
		}

		http.NotFound(w, r)
	}
}
