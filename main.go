package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/drive/v3"
	"gopkg.in/yaml.v2"

	"github.com/graphaelli/zat/cmd"
	"github.com/graphaelli/zat/google"
	"github.com/graphaelli/zat/zoom"
)

func NewMux(logger *log.Logger, googleClient *google.Client, zoomClient *zoom.Client) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		if !googleClient.HasCreds() {
			logger.Print("no google credentials")
			w.Write([]byte("<br><a href=\"/google\">google</a>"))
			// googleClient.OauthRedirect(w, r)
			//return
		}

		if !zoomClient.HasCreds() {
			logger.Print("no zoom credentials")
			w.Write([]byte("<br><a href=\"/zoom\">zoom</a>"))
			//zoomClient.OauthRedirect(w, r)
			//return
		}
	})

	mux.HandleFunc("/google", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/google" {
			http.NotFound(w, r)
			return
		}

		if !googleClient.HasCreds() {
			logger.Print("no google credentials, redirecting")
			googleClient.OauthRedirect(w, r)
			return
		}

		query := fmt.Sprintf("mimeType='%s'", google.MimeTypeFolder)
		andQuery := r.FormValue("q")
		if andQuery != "" {
			query += " and " + andQuery
		}
		files, err := googleClient.ListFiles(context.TODO(), query, "")
		if err != nil {
			logger.Print(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(files); err != nil {
			logger.Print(err)
		}
	})
	mux.HandleFunc("/zoom", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zoom" {
			http.NotFound(w, r)
			return
		}

		if !zoomClient.HasCreds() {
			logger.Print("no zoom credentials, redirecting")
			zoomClient.OauthRedirect(w, r)
			return
		}

		recordings, err := zoomClient.ListRecordings(time.Now().Add(-168 * time.Hour), "")
		if err != nil {
			logger.Print(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if recordings == nil {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(recordings); err != nil {
			logger.Print(err)
		}
	})

	mux.HandleFunc("/oauth/google", googleClient.OauthHandler())
	mux.HandleFunc("/oauth/zoom", zoomClient.OauthHandler())
	return mux
}

type Directive struct {
	Name   string `json:"name"`
	Google string `json:"google"`
	Zoom   string `json:"zoom"`
}

// zoom meeting -> action
type Config struct {
	logger       *log.Logger
	copies       map[int64]string // for now
	googleClient *google.Client
	zoomClient   *zoom.Client
}

func NewConfigFromFile(logger *log.Logger, path string, googleClient *google.Client, zoomClient *zoom.Client) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return NewConfigFromReader(logger, f, googleClient, zoomClient)
}

func NewConfigFromReader(logger *log.Logger, r io.Reader, googleClient *google.Client, zoomClient *zoom.Client) (*Config, error) {
	var directives []Directive
	if err := yaml.NewDecoder(r).Decode(&directives); err != nil {
		return nil, err
	}
	c := make(map[int64]string, 0)
	for _, d := range directives {
		key, err := strconv.ParseInt(strings.ReplaceAll(d.Zoom, "-", ""), 10, 64)
		if err != nil {
			return nil, err
		}
		if _, exists := c[key]; exists {
			logger.Printf("config for %d already exists, disabling any action", key)
			c[key] = "skip"
			continue
		}
		c[key] = d.Google
	}
	return &Config{
		logger:       logger,
		copies:       c,
		googleClient: googleClient,
		zoomClient:   zoomClient,
	}, nil
}

// meetingFolderName constructs the name of the gdrive folder containing the meeting
func meetingFolderName(meeting zoom.Meeting) string {
	// TODO: allow customization of folder name, perhaps "docker inspect --format" style
	return meeting.StartTime.Format("2006-01-02")
}

// recordingFileName constructs the name of the file for this meeting
func recordingFileName(meeting zoom.Meeting, recording zoom.RecordingFile) string {
	// TODO: allow customization of file name, perhaps "docker inspect --format" style
	start, err := time.Parse(time.RFC3339, recording.RecordingStart)
	if err != nil {
		start = meeting.StartTime
	}
	baseName := fmt.Sprintf("%s %s", start.Format("2006-01-02-150405"), meeting.Topic)
	var ext string
	switch e := strings.ToLower(recording.FileType); e {
	case "chat":
		ext = "chat.log"
	case "m4a", "mp4":
		ext = e
	case "timeline":
		ext = "timeline.json"
	case "transcript":
		ext = "vtt"
	default:
		if recording.RecordingType != "" {
			ext = strings.ToLower(recording.RecordingType) + "." + e
		} else {
			ext = e
		}
	}
	return baseName + "." + ext
}

func mkdir(ctx context.Context, gdrive *drive.Service, parent *drive.File, folder string) (*drive.File, error, bool) {
	// maybe no need to check if it exists first, can just "mkdir -p" no matter what? for now look to enable dryrun
	// exact match 1 folder
	query := fmt.Sprintf("mimeType=%q and %q in parents and name=%q and trashed=false", google.MimeTypeFolder, parent.Id, folder)
	if result, err := gdrive.Files.List().SupportsTeamDrives(true).IncludeTeamDriveItems(true).Q(query).Do(); err != nil {
		return nil, err, false
	} else {
		fileCount := len(result.Files)
		if fileCount > 1 {
			return nil, fmt.Errorf("%d files found: %#v, expected 0 or 1", fileCount, result.Files), false
		} else if fileCount == 1 {
			return result.Files[0], nil, false
		}
	}

	// folder doesn't exist when we checked, create it.  no real problem if it was already created
	if result, err := gdrive.Files.Create(&drive.File{
		Name:     folder,
		MimeType: google.MimeTypeFolder,
		Parents:  []string{parent.Id},
	}).SupportsAllDrives(true).Do(); err != nil {
		return nil, err, false
	} else {
		return result, nil, true
	}
}

func (z *Config) Archive(meeting zoom.Meeting) error {
	// check what is already uploaded for this meeting
	gdrive, err := z.googleClient.Service(context.TODO())
	if err != nil {
		return fmt.Errorf("while creating gdrive client: %w", err)
	}
	// parent folder of all meetings
	parentFolderName := z.copies[meeting.ID]
	if parentFolderName == "" {
		return fmt.Errorf("no mapping found for meeting %d", meeting.ID)
	}
	parent, err := gdrive.Files.Get(parentFolderName).SupportsAllDrives(true).Do()
	if err != nil {
		return fmt.Errorf("while finding parent of %q: %w", parentFolderName, err)
	}

	// parent folder for this meeting
	meetingFolder, err, created := mkdir(context.TODO(), gdrive, parent, meetingFolderName(meeting))
	if err != nil {
		return fmt.Errorf("while finding/creating meeting folder: %w", err)
	}
	if created {
		z.logger.Printf("created folder %s: https://drive.google.com/drive/folders/%s",
			meetingFolder.Name, meetingFolder.Id)
	} else {
		z.logger.Printf("using existing folder %s: https://drive.google.com/drive/folders/%s",
			meetingFolder.Name, meetingFolder.Id)
	}

	// list folder for this meeting
	alreadyUploaded := make(map[string]struct{})
	nextPageToken := ""
	for page := 0; page < 5; page++ {
		call := gdrive.Files.List().SupportsTeamDrives(true).IncludeTeamDriveItems(true).Q(fmt.Sprintf("%q in parents", meetingFolder.Id))
		if nextPageToken != "" {
			call = call.PageToken(nextPageToken)
		}
		meetingFiles, err := call.Do()
		if err != nil {
			return fmt.Errorf("while listing meeting folder: %w", err)
		}
		for _, f := range meetingFiles.Files {
			alreadyUploaded[f.Name] = struct{}{}
		}
		if meetingFiles.NextPageToken == "" {
			break
		}
		nextPageToken = meetingFiles.NextPageToken
	}

	// download & upload serially for now
	z.logger.Printf("archiving meeting %d to %s (https://drive.google.com/drive/folders/%s)",
		meeting.ID, meetingFolder.Name, meetingFolder.Id)
	for _, f := range meeting.RecordingFiles {
		name := recordingFileName(meeting, f)
		if _, exists := alreadyUploaded[name]; exists {
			z.logger.Printf("skipping upload %s to %s/%s, already exists", name, parent.Name, meetingFolder.Name)
			continue
		}
		z.logger.Printf("uploading %q to \"%s/%s\"", name, parent.Name, meetingFolder.Name)
		r, err := http.Get(f.DownloadURL)
		if err != nil {
			return fmt.Errorf("while downloading recording %s: %w", f.DownloadURL, err)
		}
		if r.StatusCode != http.StatusOK {
			return fmt.Errorf("download failed, got %d error: %#v", r.StatusCode, r)
		}
		_, err = gdrive.Files.Create(&drive.File{
			Name:    name,
			Parents: []string{meetingFolder.Id},
		}).Media(r.Body).SupportsAllDrives(true).Do()
		if err != nil {
			return fmt.Errorf("while uploading recording %s: %w", f.DownloadURL, err)
		}
		z.logger.Printf("uploaded %q to %s/%s", name, parent.Name, meetingFolder.Name)
	}
	return nil
}

type runParams struct {
	minDuration int
	since       time.Duration
}

func (z *Config) Run(params runParams) error {
	z.logger.Print("archiving recordings")
	nextPageToken := ""
	for {
		recordings, err := z.zoomClient.ListRecordings(time.Now().Add(-1 * params.since), nextPageToken)
		if err != nil {
			return fmt.Errorf("failed to list recordings: %w", err)
		}
		for _, meeting := range recordings.Meetings {
			if meeting.Duration < params.minDuration {
				z.logger.Printf("skipped %d minute meeting at %s", meeting.Duration, meeting.StartTime)
				continue
			}
			if err := z.Archive(meeting); err != nil {
				z.logger.Print(err)
			}
		}
		nextPageToken = recordings.NextPageToken
		if nextPageToken == "" {
			break
		}
	}
	z.logger.Print("done archiving recordings")
	return nil
}

func main() {
	cfgDir := cmd.FlagConfigDir()
	addr := flag.String("addr", "localhost:8080", "web server listener address")
	noServer := flag.Bool("no-server", false, "don't start web server")
	minDuration := flag.Int("min-duration", 5, "minimum meeting duration in minutes to archive")
	since := flag.Duration("since", 168*time.Hour, "since")
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
	zoomClient, err := zoom.NewClientFromFile(
		logger,
		path.Join(*cfgDir, cmd.ZoomConfigPath),
		zoom.NewCredentialsManager(cmd.ZoomCredsPath).ClientOption,
	)
	if err != nil {
		logger.Fatal(err)
	}

	var wg sync.WaitGroup
	if ! *noServer {
		wg.Add(1)
		server := http.Server{
			Addr:    *addr,
			Handler: NewMux(logger, googleClient, zoomClient),
		}
		go func() {
			logger.Printf("starting on http://%s", server.Addr)
			if err := server.ListenAndServe(); err != nil {
				logger.Fatal(err)
			}
			wg.Done()
		}()
	}

	zat, err := NewConfigFromFile(logger, cmd.ZatConfigPath, googleClient, zoomClient)
	if err != nil {
		logger.Println("failed to load config", err)
	}

	if ! googleClient.HasCreds() {
		logger.Println("no google creds")
	}
	if ! zoomClient.HasCreds() {
		logger.Println("no zoom creds")
	}

	if zat != nil {
		wg.Add(1)
		go func() {
			rp := runParams{
				minDuration: *minDuration,
				since:       *since,
			}
			logger.Print("starting archive tool")
			if err := zat.Run(rp); err != nil {
				logger.Println(err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
}
