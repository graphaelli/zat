package zoom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"golang.org/x/oauth2"
)

const (
	timeQuery = "2006-01-02"
)

// TODO: RecordingStart RecordingEnd time.Time (currently can be empty and blows up unmarshal)
type RecordingFile struct {
	ID             string `json:"id,omitempty"`
	MeetingID      string `json:"meeting_id"`
	RecordingStart string `json:"recording_start"`
	RecordingEnd   string `json:"recording_end"`
	FileType       string `json:"file_type"`
	FileSize       int    `json:"file_size,omitempty"`
	PlayURL        string `json:"play_url,omitempty"`
	DownloadURL    string `json:"download_url"`
	Status         string `json:"status,omitempty"`
	RecordingType  string `json:"recording_type,omitempty"`
}

type Meeting struct {
	UUID           string          `json:"uuid"`
	ID             int64           `json:"id"`
	AccountID      string          `json:"account_id"`
	HostID         string          `json:"host_id"`
	Topic          string          `json:"topic"`
	Type           int             `json:"type"`
	StartTime      time.Time       `json:"start_time"`
	Timezone       string          `json:"timezone"`
	Duration       int             `json:"duration"`
	TotalSize      int             `json:"total_size"`
	RecordingCount int             `json:"recording_count"`
	ShareURL       string          `json:"share_url"`
	RecordingFiles []RecordingFile `json:"recording_files"`
}

type ListRecordingsResponse struct {
	From          string    `json:"from"`
	To            string    `json:"to"`
	PageCount     int       `json:"page_count"`
	PageSize      int       `json:"page_size"`
	TotalRecords  int       `json:"total_records"`
	NextPageToken string    `json:"next_page_token"`
	Meetings      []Meeting `json:"meetings"`
}

type Client struct {
	logger     *log.Logger
	httpClient *http.Client

	apiBaseUrl  *url.URL
	config      *oauth2.Config
	credentials *oauth2.Token
	cm          *credentialsManager
}

type ClientOption func(*Client)

func CustomHTTPClientOption(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

type Config struct {
	Id            string `json:"id"`
	Secret        string `json:"secret"`
	OauthRedirect string `json:"oauth_redirect"`
	ApiBaseUrl    string `json:"api_url"`
	AuthUrl       string `json:"auth_url"`
	TokenUrl      string `json:"token_url"`
}

func NewClientFromFile(logger *log.Logger, path string, options ...ClientOption) (*Client, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return NewClientFromReader(logger, f, options...)
}

func NewClientFromReader(logger *log.Logger, r io.Reader, options ...ClientOption) (*Client, error) {
	var config Config
	if err := json.NewDecoder(r).Decode(&config); err != nil {
		return nil, err
	}
	return NewClient(logger, config, options...)
}

func NewClient(logger *log.Logger, config Config, options ...ClientOption) (*Client, error) {
	if config.Id == "" || config.Secret == "" || config.OauthRedirect == "" {
		return nil, errors.New("configuration requires id, secret, oauth redirect")
	}

	if config.ApiBaseUrl == "" {
		config.ApiBaseUrl = "https://api.zoom.us/"
	}
	if config.AuthUrl == "" {
		config.AuthUrl = "https://zoom.us/oauth/authorize"
	}
	if config.TokenUrl == "" {
		config.TokenUrl = "https://zoom.us/oauth/token"
	}

	apiUrl, err := url.Parse(config.ApiBaseUrl)
	if err != nil {
		return nil, err
	}

	c := &Client{
		logger:     logger,
		httpClient: http.DefaultClient,

		apiBaseUrl: apiUrl,
		config: &oauth2.Config{
			ClientID:     config.Id,
			ClientSecret: config.Secret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  config.AuthUrl,
				TokenURL: config.TokenUrl,
			},
			RedirectURL: config.OauthRedirect,
		},
	}

	for _, o := range options {
		o(c)
	}
	return c, nil
}

func (c *Client) updateCreds(token *oauth2.Token) {
	c.credentials = token
	if token != nil && c.cm != nil {
		if err := c.cm.saveCreds(c); err != nil {
			c.logger.Println("failed to save creds:", err)
		}
	}
}

func (c *Client) HasCreds() bool {
	return c.credentials.Valid()
}

func (c *Client) OauthRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, c.config.RedirectURL, http.StatusFound)
}

func (c *Client) addBearerAuth(r *http.Request) {
	r.Header.Set("Authorization", "Bearer "+c.credentials.AccessToken)
}

func (c *Client) NewApiRequest(method, uri string) (*http.Request, error) {
	u := *c.apiBaseUrl
	u.Path = path.Join(u.Path, uri)
	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.HasCreds() {
		c.addBearerAuth(req)
	}
	return req, nil
}

func (c *Client) Do(req *http.Request, decodeTo interface{}) (*http.Response, error) {
	rsp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("while creating http client in Do: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("API call to %s failed: %d", req.URL.String(), rsp.StatusCode)
		if body, err := ioutil.ReadAll(rsp.Body); err == nil {
			msg += ": " + string(body)
		}
		c.logger.Print(msg)
		return nil, errors.New(msg)
	}

	d := json.NewDecoder(rsp.Body)
	if err := d.Decode(&decodeTo); err != nil {
		return nil, fmt.Errorf("while decoding response in Do: %w", err)
	}
	return rsp, nil
}

func (c *Client) OauthHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("refresh") != "" || (c.credentials != nil && c.credentials.Expiry.Before(time.Now())) {
			c.updateCreds(nil)
		}

		code := r.FormValue("code")
		if code == "" {
			redirectTo := c.config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
			c.logger.Print("no code provided, sending to zoom auth ", redirectTo)
			http.Redirect(w, r, redirectTo, http.StatusFound)
			return
		}

		// exchange for token
		if token, err := c.config.Exchange(context.TODO(), code); err != nil {
			c.logger.Print(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			c.updateCreds(token)
		}

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// TODO: accept ListRecordingsRequest
// TODO: allow setting "to" to address > 30 day spans
// https://marketplace.zoom.us/docs/api-reference/zoom-api/cloud-recording/recordingslist
func (c *Client) ListRecordings(since time.Time, nextPageToken string) (*ListRecordingsResponse, error) {
	var j ListRecordingsResponse
	req, err := c.NewApiRequest(http.MethodGet, "v2/users/me/recordings")
	if err != nil {
		return nil, fmt.Errorf("while building ListRecordings request: %w", err)
	}
	v := req.URL.Query()
	v.Set("from", since.Format(timeQuery))
	v.Set("page_size", "300") // max
	if nextPageToken != "" {
		v.Set("next_page_token", nextPageToken)
	}
	req.URL.RawQuery = v.Encode()
	if _, err := c.Do(req, &j); err != nil {
		return nil, fmt.Errorf("while executing ListRecordings request: %w", err)
	}
	return &j, nil
}

func (c *Client) UpdateOauthRedirect(url string) {
	c.config.RedirectURL = url
}
