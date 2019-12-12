package zoom

import (
	"bytes"
	"encoding/json"
	"log"
	"testing"
)

func TestNewClientFromReader(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		validate func(*testing.T, *Client, error)
		wantErr  bool
	}{
		{
			name:   "Nil",
			config: nil,
			validate: func(t *testing.T, c *Client, err error) {
				if err == nil {
					t.Error("expected error from nil config")
				}
			},
		},
		{
			name:   "Empty",
			config: &Config{},
			validate: func(t *testing.T, c *Client, err error) {
				if err == nil {
					t.Error("expected error from empty config")
				}
			},
		},
		{
			name: "Incomplete",
			config: &Config{
				Id: "test-id",
			},
			validate: func(t *testing.T, c *Client, err error) {
				if err == nil {
					t.Error("expected error from incomplete config")
				}
			},
		},
		{
			name: "Minimal",
			config: &Config{
				Id:            "test-id",
				Secret:        "test-secret",
				OauthRedirect: "http://redirect",
			},
			validate: func(t *testing.T, c *Client, err error) {
				if err != nil {
					t.Error("expected no error from minimal config, got:", err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config bytes.Buffer
			if err := json.NewEncoder(&config).Encode(tt.config); err != nil {
				t.Error(err)
			}
			var clog bytes.Buffer
			got, err := NewClientFromReader(log.New(&clog, "", 0), &config)
			tt.validate(t, got, err)
		})
	}
}
