package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/graphaelli/zat/cmd"
	"github.com/graphaelli/zat/google"
)

func main() {
	andQuery := flag.String("query", "", "google drive query: https://developers.google.com/drive/api/v3/search-files")
	cfgDir := cmd.FlagConfigDir()
	flag.Parse()

	logger := log.New(os.Stderr, "", cmd.LogFmt)
	googleClient, err := google.NewClientFromFile(
		logger,
		path.Join(*cfgDir, cmd.GoogleConfigPath),
		google.NewCredentialsManager(cmd.GoogleCredsPath).ClientOption,
	)
	if err != nil {
		logger.Fatal(err)
	}
	query := fmt.Sprintf("mimeType='%s'", google.MimeTypeFolder)
	if *andQuery != "" {
		query += " and " + *andQuery
	}
	var pageToken string
	for page := 1; page < 5; page++ {
		files, err := googleClient.ListFiles(context.TODO(), query, pageToken)
		if err != nil {
			logger.Fatal(err)
		}
		for _, f := range files.Files {
			name := f.Name
			if len(name) >= 60 {
				name = name[0:59] + "⋯"
			}
			fmt.Printf("%-60s %-33s https://drive.google.com/drive/folders/%s\n", name, f.Id, f.Id)
		}
		if files.NextPageToken == "" {
			break
		}
		pageToken = files.NextPageToken
	}
}
