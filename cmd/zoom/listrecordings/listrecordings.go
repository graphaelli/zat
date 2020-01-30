package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"sort"
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
		fmt.Printf("%s %d %q\n", meeting.StartTime.Format("2006-01-02"), meeting.ID, meeting.Topic)
		recs := make(map[string]*zoom.RecordingFile, len(meeting.RecordingFiles))
		recTypes := make([]string, 0, len(meeting.RecordingFiles))
		for i, f := range meeting.RecordingFiles {
			typ := f.RecordingType
			if typ == "" {
				typ = f.FileType
			}
			uniqType := fmt.Sprintf("%s|%d", strings.ToLower(typ), i)
			capture := f
			recs[uniqType] = &capture
			recTypes = append(recTypes, uniqType)
		}
		sort.Strings(recTypes)
		for _, typ := range recTypes {
			fmt.Printf("  %-32s %s\n", typ[:strings.LastIndex(typ, "|")], recs[typ].DownloadURL)
		}
	}
}
