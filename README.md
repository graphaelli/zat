# Zoom Archive Tool

Copies Zoom recordings to Google Drive.

## Why

* Improve discoverability of Zoom recordings
* Extend retention longer than Zoom's 6 months
* Preserve recordings when Zoom user leaves the org

## How

`zat` requires `go` to build - 1.13+ is recommended, for proper error formatting, but will work with older versions.

```
$ go build .
$ ./zat
```

`zat` should start without any configuration but isn't very useful without credentials - see below for setup.

`zat` will persist tokens to disk at `google.creds.json` and `zat.creds.json` - be sure to guard those files carefully as permissions are neceesarily wide.

_note: zoom credentials do not automatically refresh yet, `zat` will require re-authorization after an hour.
Until this is resolved, you will need to run an instance of the zat web server before each zat archival.

Once tokens have been obtained, `zat -no-server` will perform only archival duties.

`zat` always attempts to archive, to only start the web server use: `-since 0s`.

Zoom does somethng funny when `-since` is > 30 days, there is a todo for that.

### Credentials

* Obtain Google credentials
  * [Create an Oauth Client ID credential](https://console.cloud.google.com/apis/credentials) _note: you may need to create a new project first_
    * Set Authorized redirect URIs to `http://localhost:8080/oauth/google`
  * Save credentials to `google.config.json` (GCP Console > API & Services > Credentials > Download JSON)

* Obtain Zoom credentials
  * [Create an Oauth Application](https://marketplace.zoom.us/develop/create)
    * User-managed
    * No need to publish
    * Set redirect URI to `http://127.0.0.1.ip.es.io:8080/oauth/zoom`
  * Saved credentials to `zoom.config.json` with content:
```json
{
  "id":             "your-id",
  "secret":         "your-secret",
  "oauth_redirect": "http://127.0.0.1.ip.es.io:8080/oauth/zoom"
}
```

### Configuration

* Configure zat

Create zat.yml like:

```yaml
- name: UI Weekly
  google: DpB3XhhzV87LfEeLrM-nCopTtHDWxqVGH
  zoom: 023-456-789
- name: Team Weekly
  google: DpB3XhhzV87LfEeLrM-nCopTtHDWxqVGH
  zoom: 123-456-789
```

#### Google

The google configuration is the ID of the folder where the recordings will be stored.

`cmd/google/findfolders` can assist in tracking down folders and IDs like:

```
$ go build ./cmd/google/findfolders
$ ./findfolders -query 'name = "bar"'
foo                                                          DpB3XhhzV87LfEeLrM-nCopTtHDWxqVGH https://drive.google.com/drive/folders/DpB3XhhzV87LfEeLrM-nCopTtHDWxqVGH
$ ./findfolders -query '"DpB3XhhzV87LfEeLrM-nCopTtHDWxqVGH" in parents and name = "Meetings"'
Meetings                                                     ycMAKmDuzwobv6eBf9-PLupEGJJ6BtyoJ https://drive.google.com/drive/folders/ycMAKmDuzwobv6eBf9-PLupEGJJ6BtyoJ
```

zat provides a web interface with similar functionality, eg [http://localhost:8080/google?q=name contains "Team weekly"](http://localhost:8080/google?q=name%20contains%20%27Team%20weekly%27).

#### Zoom

The zoom configuration is the meeting ID - the dashes are optional.

`cmd/zoom/listrecordings` can assist in tracking down meeting IDs like:

```
$ go build ./cmd/zoom/listrecordings
$ $ ./listrecordings -since 96h
2019/11/25 12:22:54 listrecordings.go:41: 2 recordings found
2019-11-21 945106202 UI Weekly
	audio_transcript https://zoom.us/recording/download/wwww
	shared_screen_with_speaker_view https://zoom.us/recording/download/xxxx
	chat_file https://zoom.us/recording/download/yyyy
	audio_only https://zoom.us/recording/download/zzzz
	timeline https://zoom.us/recording/download/11111111-1111-1111-1111-111111111111
2019-11-21 906290321 Team Weekly
	audio_transcript https://zoom.us/recording/download/aaaa
	shared_screen_with_speaker_view https://zoom.us/recording/download/bbbb
	chat_file https://zoom.us/recording/download/cccc
	audio_only https://zoom.us/recording/download/dddd
	timeline https://zoom.us/recording/download/22222222-2222-2222-2222-222222222222
```

zat provides a web interface with similar functionality at http://localhost:8080/zoom.
