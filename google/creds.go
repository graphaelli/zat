package google

import (
	"encoding/json"
	"os"

	"golang.org/x/oauth2"
)

type credentialsManager struct {
	path string
}

func NewCredentialsManager(path string) *credentialsManager {
	return &credentialsManager{path: path}
}

func (cm *credentialsManager) loadCreds(c *Client) error {
	f, err := os.Open(cm.path)
	if err != nil {
		return err
	}
	defer f.Close()
	var token oauth2.Token
	if err := json.NewDecoder(f).Decode(&token); err != nil {
		return err
	}
	c.credentials = &token
	return nil
}

func (cm *credentialsManager) saveCreds(c *Client) error {
	f, err := os.OpenFile(cm.path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(c.credentials)
}

func (cm *credentialsManager) ClientOption(c *Client) {
	c.cm = cm
	if err := c.cm.loadCreds(c); err != nil {
		c.logger.Println("failed to load creds:", err)
	}
}
