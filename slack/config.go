package slack

import (
	"encoding/json"
	"io"
	"log"
	"os"

	slackapi "github.com/slack-go/slack"
)

type Config struct {
	Token string `json:"token"`
}

func NewClientFromFile(logger *log.Logger, path string, options ...slackapi.Option) (*slackapi.Client, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return NewClientFromReader(logger, f, options...)
}

func NewClientFromReader(logger *log.Logger, r io.Reader, options ...slackapi.Option) (*slackapi.Client, error) {
	var config Config
	if err := json.NewDecoder(r).Decode(&config); err != nil {
		return nil, err
	}
	return NewClient(logger, config, options...)
}

func NewClientFromEnv(logger *log.Logger, options ...slackapi.Option) (*slackapi.Client, error) {
	token := os.Getenv("ZAT_SLACK_TOKEN")
	if token != "" {
		return NewClient(logger, Config{Token: token}, options...)
	}
	return nil, nil
}

func NewClient(logger *log.Logger, config Config, options ...slackapi.Option) (*slackapi.Client, error) {
	return slackapi.New(config.Token, options...), nil
}

// NewClientFromEnvOrFile is a convenience function for getting a slack client from the env if possible, and
// otherwise from the default location, and otherwise just staying quiet
func NewClientFromEnvOrFile(logger *log.Logger, path string, options ...slackapi.Option) (*slackapi.Client, error) {
	if client, _ := NewClientFromEnv(logger, options...); client != nil {
		return client, nil
	}
	client, err := NewClientFromFile(logger, path, options...)
	if err == nil {
		return client, nil
	}
	if !os.IsNotExist(err) {
		logger.Printf("failed to load slack configuration: %s, continuing", err)
	}
	return nil, nil

}
