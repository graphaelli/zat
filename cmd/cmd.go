package cmd

import (
	"flag"
	"log"
)

const (
	LogFmt = log.Ldate | log.Ltime | log.Lshortfile

	// google API credentials - oath2.Config{}, read only ok
	GoogleConfigPath = "google.config.json"
	// zoom OAuth persistence - oauth2.Token{}, read/write
	GoogleCredsPath = "google.creds.json"
	// zoom API credentials - zoom.Config{}, read only ok
	ZoomConfigPath = "zoom.config.json"
	// zoom OAuth persistence - oauth2.Token{}, read/write
	ZoomCredsPath = "zoom.creds.json"

	ZatConfigPath = "zat.yml"
)

func FlagConfigDir() *string {
	return flag.String("config-dir", ".", "base directory for configuration files")
}
