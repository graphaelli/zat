package main

import (
	"bytes"
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

	slackapi "github.com/slack-go/slack"
	"go.elastic.co/apm"
	"go.elastic.co/apm/module/apmhttp"
	"google.golang.org/api/drive/v3"
	"gopkg.in/yaml.v2"

	"github.com/graphaelli/zat/cmd"
	"github.com/graphaelli/zat/google"
	"github.com/graphaelli/zat/slack"
	"github.com/graphaelli/zat/zoom"
)

// mustWriter is an io.Writer that panics on Write error
// for use where panics are recovered eg http handler
type mustWriter struct {
	w io.Writer
}

func (m mustWriter) Write(b []byte) {
	if _, err := m.w.Write(b); err != nil {
		panic(err)
	}
}

func NewMux(zat *Config, params runParams) *http.ServeMux {
	logger := zat.logger
	googleClient := zat.googleClient
	zoomClient := zat.zoomClient

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		mw := mustWriter{w: w}
		w.Header().Set("Content-Type", "text/html")
		mw.Write([]byte("<meta http-equiv=\"refresh\" content=\"10\"/>"))
		mw.Write([]byte("<head><style>table, table th,table tr, table td{border-collapse: collapse;border:1px solid #000000;padding:3px}</style></head><body>"))

		mw.Write([]byte("<br>Google: "))
		if !googleClient.HasCreds() {
			mw.Write([]byte("<a href=\"/google\">login</a>"))
			// googleClient.OauthRedirect(w, r)
			//return
		} else {
			mw.Write([]byte("<span style=\"color:green\">OK</span>"))
		}

		mw.Write([]byte("<br>Zoom: "))
		if !zoomClient.HasCreds() {
			mw.Write([]byte("<a href=\"/zoom\">login</a>"))
			//zoomClient.OauthRedirect(w, r)
			//return
		} else {
			mw.Write([]byte("<span style=\"color:green\">OK</span>"))
		}

		if archIsRunning {
			mw.Write([]byte("<br/>Archiving...</a>"))
		} else if googleClient.HasCreds() && zoomClient.HasCreds() {
			mw.Write([]byte("<br/><a href=\"/archive\">Archive Now</a>"))
		} else {
			mw.Write([]byte("<br/>Login, to be able to archive"))
		}

		if len(archDetails) > 0 {
			mw.Write([]byte("<br/><br/><table><tr><th>Name</th><th>Date</th><th>Files</th><th>Status</th></th>"))
			for i := 0; i < len(archDetails); i++ {
				arch := archDetails[i]
				mw.Write([]byte(fmt.Sprintf("<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td>%d</td><td><a href=\"%s\">%s</a></td></tr>",
					arch.zoomUrl, arch.name, arch.date, arch.fileNumber, arch.googleDriveURL, arch.status)))
			}
			mw.Write([]byte("</table>"))
		}
		mw.Write([]byte("</body>"))
	})

	mux.HandleFunc("/archive", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/archive" {
			http.NotFound(w, r)
			return
		}

		go doRun(zat, params)
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
		files, err := googleClient.ListFiles(r.Context(), query, "")
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

		recordings, err := zoomClient.ListRecordings(r.Context(), time.Now().Add(-168*time.Hour), "")
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
	Slack  string `json:"slack"`
}

// use invalid json to avoid conflict
var skipDirective = Directive{Name: "{skip"}

// zoom meeting -> action
type Config struct {
	logger       *log.Logger
	copies       map[int64]Directive
	googleClient *google.Client
	slackClient  *slackapi.Client
	zoomClient   *zoom.Client
}

func NewConfigFromFile(logger *log.Logger, path string, googleClient *google.Client, zoomClient *zoom.Client,
	slackClient *slackapi.Client) (*Config, error) {
	f, err := os.Open(path)

	if err != nil && os.IsExist(err) {
		return nil, err
	}

	var r io.Reader = f

	// Use an empty io.Reader when the files doesn't exist on disk.
	if f == nil {
		r = bytes.NewReader(nil)
	} else {
		defer f.Close()
	}

	return NewConfigFromReader(logger, r, googleClient, zoomClient, slackClient)
}

func NewConfigFromReader(logger *log.Logger, r io.Reader, googleClient *google.Client, zoomClient *zoom.Client,
	slackClient *slackapi.Client) (*Config, error) {
	var directives []Directive
	if err := yaml.NewDecoder(r).Decode(&directives); err != nil && err != io.EOF {
		return nil, err
	}
	c := map[int64]Directive{}
	for _, d := range directives {
		key, err := strconv.ParseInt(strings.ReplaceAll(d.Zoom, "-", ""), 10, 64)
		if err != nil {
			return nil, err
		}
		if _, exists := c[key]; exists {
			logger.Printf("config for %d already exists, disabling any action", key)
			c[key] = skipDirective
			continue
		}
		c[key] = d
	}
	return &Config{
		logger:       logger,
		copies:       c,
		googleClient: googleClient,
		slackClient:  slackClient,
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
	span, ctx := apm.StartSpan(ctx, "mkdir", "app")
	defer span.End()

	// maybe no need to check if it exists first, can just "mkdir -p" no matter what? for now look to enable dryrun
	// exact match 1 folder
	query := fmt.Sprintf("mimeType=%q and %q in parents and name=%q and trashed=false", google.MimeTypeFolder, parent.Id, folder)
	if result, err := gdrive.Files.List().Context(ctx).SupportsTeamDrives(true).IncludeTeamDriveItems(true).Q(query).Do(); err != nil {
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
	}).Context(ctx).SupportsAllDrives(true).Do(); err != nil {
		return nil, err, false
	} else {
		return result, nil, true
	}
}

func (z *Config) Archive(ctx context.Context, meeting zoom.Meeting, params runParams) error {
	span, ctx := apm.StartSpan(ctx, "Archive", "app")
	defer span.End()

	var curArchMeeting = archivedMeeting{name: meeting.Topic,
		fileNumber: 0,
		status:     "archiving",
		date:       meeting.StartTime.Format("2006-01-02 15:04"),
		zoomUrl:    meeting.ShareURL}
	archDetails = append(archDetails, &curArchMeeting)

	// check what is already uploaded for this meeting
	gdrive, err := z.googleClient.Service(ctx)
	if err != nil {
		return fmt.Errorf("while creating gdrive client: %w", err)
	}
	action := z.copies[meeting.ID]
	if action == skipDirective {
		curArchMeeting.status = "error"
		return fmt.Errorf("skipped mapping meeting %d %q", meeting.ID, meeting.Topic)
	}
	// parent folder of all meetings
	parentFolderName := action.Google
	if parentFolderName == "" {
		curArchMeeting.status = "error"
		return fmt.Errorf("no mapping found for meeting %d %q", meeting.ID, meeting.Topic)
	}

	parent, err := gdrive.Files.Get(parentFolderName).Context(ctx).SupportsAllDrives(true).Do()
	if err != nil {
		curArchMeeting.status = "error"
		return fmt.Errorf("while finding parent of %q: %w", parentFolderName, err)
	}

	// parent folder for this meeting
	meetingFolder, err, created := mkdir(ctx, gdrive, parent, meetingFolderName(meeting))
	if err != nil {
		curArchMeeting.status = "error"
		return fmt.Errorf("while finding/creating meeting folder: %w", err)
	}
	if created {
		z.logger.Printf("created folder %s: https://drive.google.com/drive/folders/%s",
			meetingFolder.Name, meetingFolder.Id)
	} else {
		z.logger.Printf("using existing folder %s: https://drive.google.com/drive/folders/%s",
			meetingFolder.Name, meetingFolder.Id)
	}

	curArchMeeting.googleDriveURL = "https://drive.google.com/drive/folders/" + meetingFolder.Id

	// list folder for this meeting
	alreadyUploaded := make(map[string]struct{})
	nextPageToken := ""
	for page := 0; page < 5; page++ {
		call := gdrive.Files.List().
			Context(ctx).
			SupportsTeamDrives(true).
			IncludeTeamDriveItems(true).
			Q(fmt.Sprintf("%q in parents", meetingFolder.Id))
		if nextPageToken != "" {
			call = call.PageToken(nextPageToken)
		}
		meetingFiles, err := call.Do()
		if err != nil {
			curArchMeeting.status = "error"
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
	uploaded := false

	exclude := func(string) bool { return true }
	if params.uploadFilter != "" {
		allowedFileTypes := map[string]bool{}
		for _, uf := range strings.Split(params.uploadFilter, ",") {
			allowedFileTypes[strings.ToLower(strings.TrimSpace(uf))] = true
		}
		exclude = func(fileType string) bool {
			return !allowedFileTypes[strings.ToLower(fileType)]
		}
	}

	for _, f := range meeting.RecordingFiles {
		//check if recording file duration is shorter than minimum
		start, err := time.Parse(time.RFC3339, f.RecordingStart)
		if err != nil {
			z.logger.Printf("couldn't parse file recording start %s - %s: %v", f.ID, f.RecordingStart, err)
		}

		end, err2 := time.Parse(time.RFC3339, f.RecordingEnd)
		if err2 != nil {
			z.logger.Printf("couldn't parse file recording end %s - %s: %v", f.ID, f.RecordingStart, err)
		}

		if err == nil && err2 == nil {
			duration := int(end.Sub(start).Minutes())
			if duration < params.minDuration {
				curArchMeeting.status = "skipped - length"
				z.logger.Printf("skipped %d minute recording at %s - %s", duration, start, end)
				continue
			}
		}

		name := recordingFileName(meeting, f)

		if exclude(f.FileType) {
			z.logger.Printf("skipping upload %s, file type %q excluded", name, strings.ToLower(f.FileType))
			continue
		}

		if _, exists := alreadyUploaded[name]; exists {
			curArchMeeting.status = "done"
			curArchMeeting.fileNumber++
			z.logger.Printf("skipping upload %s to %s/%s, already exists", name, parent.Name, meetingFolder.Name)
			continue
		}
		z.logger.Printf("uploading %q to \"%s/%s\"", name, parent.Name, meetingFolder.Name)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.DownloadURL, nil)
		if err != nil {
			curArchMeeting.status = "error"
			return fmt.Errorf("while building recording download request %s: %w", f.DownloadURL, err)
		}
		v := req.URL.Query()
		v.Add("access_token", z.zoomClient.AccessToken())
		req.URL.RawQuery = v.Encode()
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			curArchMeeting.status = "error"
			return fmt.Errorf("while downloading recording %s: %w", f.DownloadURL, err)
		}
		defer r.Body.Close()

		if r.StatusCode != http.StatusOK {
			curArchMeeting.status = "error"
			return fmt.Errorf("while downloading recording %s: download failed, got %d error: %#v",
				f.DownloadURL, r.StatusCode, r)
		}
		if contentType := r.Header.Get("content-type"); strings.HasPrefix(contentType, "text/html") {
			curArchMeeting.status = "error"
			return fmt.Errorf("while downloading recording %s: download failed, got %s content",
				f.DownloadURL, contentType)
		}
		_, err = gdrive.Files.Create(&drive.File{
			Name:    name,
			Parents: []string{meetingFolder.Id},
		}).Context(ctx).Media(r.Body).SupportsAllDrives(true).Do()
		if err != nil {
			curArchMeeting.status = "error"
			return fmt.Errorf("while uploading recording %s: %w", f.DownloadURL, err)
		}
		curArchMeeting.fileNumber++
		z.logger.Printf("uploaded %q to %s/%s", name, parent.Name, meetingFolder.Name)
		uploaded = true
	}
	if uploaded && action.Slack != "" && z.slackClient != nil {
		slackSpan, ctx := apm.StartSpan(ctx, "slack", "app")
		body := fmt.Sprintf("%s recording now available: https://drive.google.com/drive/folders/%s", meeting.Topic, meetingFolder.Id)
		channel, _, text, err := z.slackClient.SendMessageContext(ctx, action.Slack, slackapi.MsgOptionText(body, true))
		if err != nil {
			z.logger.Printf("failed to notify slack %q: %s", action.Slack, err)
			apm.CaptureError(ctx, err).Send()
		} else {
			z.logger.Printf("notified slack %q: %s", channel, text)
		}
		slackSpan.End()
	}
	curArchMeeting.status = "done"
	return nil
}

type runParams struct {
	minDuration  int
	since        time.Duration
	uploadFilter string
}

func (z *Config) Run(params runParams) error {
	tx := apm.DefaultTracer.StartTransaction("archiveRecordings", "background")
	defer tx.End()
	ctx := apm.ContextWithTransaction(context.Background(), tx)

	z.logger.Print("archiving recordings")
	archDetails = []*archivedMeeting{}
	nextPageToken := ""
	for {
		recordings, err := z.zoomClient.ListRecordings(ctx, time.Now().Add(-1*params.since), nextPageToken)
		if err != nil {
			apm.CaptureError(ctx, err).Send()
			return fmt.Errorf("failed to list recordings: %w", err)
		}
		for _, meeting := range recordings.Meetings {
			if meeting.Duration < params.minDuration {
				z.logger.Printf("skipped %d minute meeting at %s", meeting.Duration, meeting.StartTime)
				continue
			}
			if err := z.Archive(ctx, meeting, params); err != nil {
				z.logger.Print(err)
				apm.CaptureError(ctx, err).Send()
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

type archivedMeeting struct {
	name           string
	fileNumber     int
	status         string
	date           string
	zoomUrl        string
	googleDriveURL string
}

var (
	archIsRunning   bool
	archIsRunningMu sync.Mutex
	archDetails     = []*archivedMeeting{}
)

func doRun(zat *Config, params runParams) {
	if zat == nil {
		// no logger to log with
		return
	}
	zat.logger.Print("starting archive tool")
	if !zat.googleClient.HasCreds() {
		zat.logger.Println("no Google creds")
		return
	}
	if !zat.zoomClient.HasCreds() {
		zat.logger.Println("no Zoom creds")
		return
	}

	archIsRunningMu.Lock()
	start := !archIsRunning
	archIsRunning = true
	archIsRunningMu.Unlock()

	if !start {
		zat.logger.Println("archiving skipped, it's already running")
		return
	}

	if err := zat.Run(params); err != nil {
		zat.logger.Println(err)
	}

	archIsRunningMu.Lock()
	archIsRunning = false
	archIsRunningMu.Unlock()
}

func main() {
	cfgDir := cmd.FlagConfigDir()
	addr := flag.String("addr", "localhost:8080", "web server listener address")
	noServer := flag.Bool("no-server", false, "don't start web server")
	minDuration := flag.Int("min-duration", 5, "minimum meeting duration in minutes to archive")
	since := flag.Duration("since", 168*time.Hour, "since")
	uploadFilter := flag.String("t", "",
		"comma separated list of file types to archive (mp4, m4a, timeline, transcript, chat, cc, csv), see: "+
			"https://marketplace.zoom.us/docs/api-reference/zoom-api/cloud-recording/recordingget")
	flag.Parse()

	logger := log.New(os.Stderr, "", cmd.LogFmt)

	// Instrument http.DefaultClient and http.DefaultTransport.
	http.DefaultClient = apmhttp.WrapClient(http.DefaultClient)
	http.DefaultTransport = apmhttp.WrapRoundTripper(http.DefaultTransport)

	googleClient, err := google.NewClientFromFile(
		logger,
		path.Join(*cfgDir, cmd.GoogleConfigPath),
		google.NewCredentialsManager(path.Join(*cfgDir, cmd.GoogleCredsPath)).ClientOption,
	)
	if err != nil {
		logger.Fatal(err)
	}
	zoomClient, err := zoom.NewClientFromFile(
		logger,
		path.Join(*cfgDir, cmd.ZoomConfigPath),
		zoom.NewCredentialsManager(path.Join(*cfgDir, cmd.ZoomCredsPath)).ClientOption,
	)
	if err != nil {
		logger.Fatal(err)
	}
	slackClient, _ := slack.NewClientFromEnvOrFile(logger, path.Join(*cfgDir, cmd.SlackConfigPath), slackapi.OptionHTTPClient(http.DefaultClient))
	rp := runParams{
		minDuration:  *minDuration,
		since:        *since,
		uploadFilter: *uploadFilter,
	}

	zat, err := NewConfigFromFile(logger, path.Join(*cfgDir, cmd.ZatConfigPath), googleClient, zoomClient, slackClient)
	if err != nil {
		// ok to continue without config, just can't do archival
		logger.Println("failed to load config", err)
	}

	var wg sync.WaitGroup
	if !*noServer {
		wg.Add(1)
		server := http.Server{
			Addr:    *addr,
			Handler: apmhttp.Wrap(NewMux(zat, rp)),
		}
		go func() {
			logger.Printf("starting on http://%s", server.Addr)
			if err := server.ListenAndServe(); err != nil {
				logger.Fatal(err)
			}
			wg.Done()
		}()
	}

	if zat != nil {
		// already logged that config is loaded, just skip the run
		wg.Add(1)
		go func() {
			doRun(zat, rp)
			wg.Done()
		}()
	}

	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	apm.DefaultTracer.Flush(ctx.Done())
}
