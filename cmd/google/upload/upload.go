package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"

	"google.golang.org/api/drive/v3"

	"github.com/graphaelli/zat/cmd"
	"github.com/graphaelli/zat/google"
)

func main() {
	cfgDir := cmd.FlagConfigDir()
	flag.Parse()

	if flag.NArg() != 2 {
		fmt.Printf("usage: %s <src> <dst>", os.Args[0])
		os.Exit(1)
	}
	logger := log.New(os.Stderr, "", cmd.LogFmt)

	src, dst := flag.Arg(0), flag.Arg(1)
	srcInfo, err := os.Stat(src)
	if err != nil {
		logger.Fatal(err)
	}
	if !srcInfo.IsDir() {
		logger.Fatal("only directories supported")
	}

	googleClient, err := google.NewClientFromFile(
		logger,
		path.Join(*cfgDir, cmd.GoogleConfigPath),
		google.NewCredentialsManager(cmd.GoogleCredsPath).ClientOption,
	)
	if err != nil {
		logger.Fatal(err)
	}

	service, err := googleClient.Service(context.TODO())
	if err != nil {
		logger.Fatal(err)
	}

	parent, err := service.Files.Get(dst).Do()
	if err != nil {
		logger.Fatal(err)
	}
	dir, err := service.Files.Create(&drive.File{
		Name:     srcInfo.Name(),
		MimeType: google.MimeTypeFolder,
		Parents:  []string{parent.Id},
	}).Do()
	if err != nil {
		logger.Fatal(err)
	}
	logger.Printf("created folder %s - https://drive.google.com/drive/folders/%s", dir.Name, dir.Id)
	files, err := ioutil.ReadDir(src)
	if err != nil {
		logger.Fatal(err)
	}

	for _, file := range files {
		r, err := os.Open(path.Join(src, file.Name()))
		if err != nil {
			logger.Fatal(err)
		}
		up, err := service.Files.Create(&drive.File{
			Name:    file.Name(),
			Parents: []string{dir.Id},
		}).Media(r).Do()

		if err != nil {
			logger.Fatal(err)
		}
		logger.Printf("uploaded %s to https://drive.google.com/drive/folders/%s", up.Name, dir.Id)
	}
}
