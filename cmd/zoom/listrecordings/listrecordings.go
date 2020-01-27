package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/graphaelli/zat/cmd"
	"github.com/graphaelli/zat/zoom"
)

func pluralize(c int) string {
	if c == 1 {
		return ""
	}
	return "s"
}

func main() {
	cfgDir := cmd.FlagConfigDir()
	since := flag.Duration("since", 168*time.Hour, "since")
	flag.Parse()

	logger := log.New(os.Stderr, "", cmd.LogFmt)
	zoomClient, err := zoom.NewClientFromFile(
		logger,
		path.Join(*cfgDir, cmd.ZoomConfigPath),
		zoom.NewCredentialsManager(cmd.ZoomCredsPath).ClientOption,
	)
	if err != nil {
		logger.Fatal(err)
	}
	recordings, err := zoomClient.ListRecordings(time.Now().Add(-1**since), "")
	if err != nil {
		logger.Fatal(err)
	}
	logger.Printf("%d recording%s found", recordings.TotalRecords, pluralize(recordings.TotalRecords))
	for _, meeting := range recordings.Meetings {
		fmt.Printf("%s %d %s\n", meeting.StartTime.Format("2006-01-02"), meeting.ID, meeting.Topic)
		for _, f := range meeting.RecordingFiles {
			typ := f.RecordingType
			if typ == "" {
				typ = f.FileType
			}
			fmt.Printf("\t%s %s\n", strings.ToLower(typ), f.DownloadURL)
		}
	}
}
