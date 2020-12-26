package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/graphaelli/zat/google"
	googlemock "github.com/graphaelli/zat/google/mock"
	"github.com/graphaelli/zat/zoom"
	zoommock "github.com/graphaelli/zat/zoom/mock"
)

var (
	nopGoogleClient = &google.Client{}
	nopZoomClient   = &zoom.Client{}
	rp              = runParams{
		minDuration: 5,
		since:       24 * time.Hour,
	}
)

func TestGoogleOauth(t *testing.T) {
	// mock google APIs and OAuth endpoints
	googleMux := http.NewServeMux()
	// googleMux.HandleFunc("/api/", googlemock.ApiHandler(t))
	googleServer := httptest.NewServer(googleMux)
	defer googleServer.Close()

	var muxBuf bytes.Buffer
	var googleBuf bytes.Buffer
	muxLog := log.New(&muxBuf, "[mux] ", 0)
	googleLog := log.New(&googleBuf, "[google] ", 0)
	googleConfig := &oauth2.Config{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		RedirectURL:  "tbd",
		Endpoint: oauth2.Endpoint{
			AuthURL:   googleServer.URL + "/oauth2/auth",
			TokenURL:  googleServer.URL + "/oauth2/token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	googleClient, err := google.NewClient(googleLog, googleConfig, google.CustomHTTPClientOption(googleServer.Client()))
	if err != nil {
		t.Error(err)
	}

	zat := &Config{
		logger:       muxLog,
		copies:       map[int64]string{},
		googleClient: googleClient,
		zoomClient:   nopZoomClient,
	}

	mux := NewMux(zat, rp)
	server := httptest.NewServer(mux)
	defer server.Close()

	// now that server has a URL, configure oauth redirect sender and handler
	oauthRedirect := server.URL + "/oauth/google"
	googleConfig.RedirectURL = oauthRedirect
	googleMux.HandleFunc("/oauth2/", googlemock.OauthHandler(t, oauthRedirect))

	client := server.Client()
	var redirects []*url.URL
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		redirects = append(redirects, req.URL)
		if len(via) >= 5 {
			return errors.New("stopped after 5 redirects")
		}
		return nil
	}
	rsp, err := client.Get(oauthRedirect)
	if err != nil {
		t.Error(err)
	}
	t.Log(strings.TrimSpace(muxBuf.String()))
	t.Log(strings.TrimSpace(googleBuf.String()))
	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected %d, got %d HTTP response", http.StatusOK, rsp.StatusCode)
	}

	expectedRedirects := []string{
		googleConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce),
		oauthRedirect + "?code=acode",
		server.URL + "/",
	}

	if len(redirects) != len(expectedRedirects) {
		t.Errorf("expected %d, got %d redirects", len(expectedRedirects), len(redirects))

	}
	for i, redirect := range redirects {
		e, err := url.Parse(expectedRedirects[i])
		if err != nil {
			t.Error(err)
		}
		if redirect.Path != e.Path || redirect.Query().Encode() != e.Query().Encode() {
			t.Errorf("expected %s, got %s in flow", e.String(), redirect.String())
		}
	}
}

func TestZoomOauth(t *testing.T) {
	// mock zoom APIs and OAuth endpoints
	zoomMux := http.NewServeMux()
	zoomMux.HandleFunc("/api/", zoommock.ApiHandler(t))
	zoomServer := httptest.NewServer(zoomMux)
	defer zoomServer.Close()

	var muxBuf bytes.Buffer
	var zoomBuf bytes.Buffer
	muxLog := log.New(&muxBuf, "[mux] ", 0)
	zoomLog := log.New(&zoomBuf, "[zoom] ", 0)
	zoomConfig := zoom.Config{
		Id:            "test-id",
		Secret:        "test-secret",
		OauthRedirect: "tbd",
		ApiBaseUrl:    zoomServer.URL + "/api",
		AuthUrl:       zoomServer.URL + "/oauth/authorize",
		TokenUrl:      zoomServer.URL + "/oauth/token",
	}
	zoomClient, err := zoom.NewClient(zoomLog, zoomConfig, zoom.CustomHTTPClientOption(zoomServer.Client()))
	if err != nil {
		t.Error(err)
	}

	zat := &Config{
		logger:       muxLog,
		copies:       map[int64]string{},
		googleClient: nopGoogleClient,
		zoomClient:   zoomClient,
	}

	mux := NewMux(zat, rp)
	server := httptest.NewServer(mux)
	defer server.Close()

	// now that server has a URL, configure oauth redirect sender and handler
	oauthRedirect := server.URL + "/oauth/zoom"
	zoomClient.UpdateOauthRedirect(oauthRedirect)
	zoomMux.HandleFunc("/oauth/", zoommock.OauthHandler(t, oauthRedirect))

	client := server.Client()
	var redirects []*url.URL
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		redirects = append(redirects, req.URL)
		if len(via) >= 5 {
			return errors.New("stopped after 5 redirects")
		}
		return nil
	}
	rsp, err := client.Get(oauthRedirect)
	if err != nil {
		t.Error(err)
	}
	t.Log(strings.TrimSpace(muxBuf.String()))
	t.Log(strings.TrimSpace(zoomBuf.String()))
	if rsp.StatusCode != http.StatusOK {
		t.Errorf("expected %d, got %d HTTP response", http.StatusOK, rsp.StatusCode)
	}

	expectedRedirects := []string{
		zoomConfig.AuthUrl + fmt.Sprintf("?access_type=offline&state=state-token&client_id=test-id&redirect_uri=%s&response_type=code", url.QueryEscape(oauthRedirect)),
		oauthRedirect + "?code=acode",
		server.URL + "/",
	}

	if len(redirects) != len(expectedRedirects) {
		t.Errorf("expected %d, got %d redirects", len(expectedRedirects), len(redirects))

	}
	for i, redirect := range redirects {
		e, err := url.Parse(expectedRedirects[i])
		if err != nil {
			t.Error(err)
		}
		if redirect.Path != e.Path || redirect.Query().Encode() != e.Query().Encode() {
			t.Errorf("expected %s, got %s in flow", e.String(), redirect.String())
		}
	}
}

func TestRecordingFileName(t *testing.T) {
	start := "2019-06-12T13:00:00Z"
	end := "2019-06-12T13:57:54Z"
	meeting := zoom.Meeting{
		Topic: "Some Meeting",
	}
	type args struct {
		meeting   zoom.Meeting
		recording zoom.RecordingFile
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "audio only",
			args: args{
				meeting: meeting,
				recording: zoom.RecordingFile{
					RecordingStart: start,
					RecordingEnd:   end,
					FileType:       "M4A",
					RecordingType:  "audio_only",
				},
			},
			want: "2019-06-12-130000 Some Meeting.m4a",
		},
		{
			name: "chat log",
			args: args{
				meeting: meeting,
				recording: zoom.RecordingFile{
					RecordingStart: start,
					RecordingEnd:   end,
					FileType:       "CHAT",
					RecordingType:  "chat_file",
				},
			},
			want: "2019-06-12-130000 Some Meeting.chat.log",
		},
		{
			name: "timeline",
			args: args{
				meeting: meeting,
				recording: zoom.RecordingFile{
					RecordingStart: start,
					RecordingEnd:   end,
					FileType:       "TIMELINE",
				},
			},
			want: "2019-06-12-130000 Some Meeting.timeline.json",
		},
		{
			name: "transcript",
			args: args{
				meeting: meeting,
				recording: zoom.RecordingFile{
					RecordingStart: start,
					RecordingEnd:   end,
					FileType:       "TRANSCRIPT",
					RecordingType:  "audio_transcript",
				},
			},
			want: "2019-06-12-130000 Some Meeting.vtt",
		},
		{
			name: "video",
			args: args{
				meeting: meeting,
				recording: zoom.RecordingFile{
					RecordingStart: start,
					RecordingEnd:   end,
					FileType:       "MP4",
					RecordingType:  "shared_screen_with_speaker_view",
				},
			},
			want: "2019-06-12-130000 Some Meeting.mp4",
		},
		{
			name: "unknown",
			args: args{
				meeting: meeting,
				recording: zoom.RecordingFile{
					RecordingStart: start,
					RecordingEnd:   end,
					FileType:       "FOO",
					RecordingType:  "bar",
				},
			},
			want: "2019-06-12-130000 Some Meeting.bar.foo",
		},
		{
			name: "missing",
			args: args{
				meeting: meeting,
				recording: zoom.RecordingFile{
					RecordingStart: start,
					RecordingEnd:   end,
					FileType:       "FOO",
				},
			},
			want: "2019-06-12-130000 Some Meeting.foo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recordingFileName(tt.args.meeting, tt.args.recording); got != tt.want {
				t.Errorf("recordingFileName() = %v, want %v", got, tt.want)
			}
		})
	}
}
