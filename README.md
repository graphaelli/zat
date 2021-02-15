# Zoom Archive Tool

Copies Zoom recordings to Google Drive.

## Why

* Improve discoverability of Zoom recordings
* Preserve recordings when Zoom user leaves the org
* Reduce cost of long term retention (maybe)

## How

`zat` requires `go` to build - 1.13+ is recommended, for proper error formatting, but will work with older versions.

```
$ go build .
$ ./zat
```

`zat` should start without any configuration but isn't very useful without credentials - see below for setup.

`zat` will persist tokens to disk at `google.creds.json` and `zat.creds.json` - be sure to guard those files carefully as permissions are necessarily wide.

Once tokens have been obtained, `zat -no-server` will perform only archival duties and then exit.

`zat` always attempts to archive, to only start the web server use: `-since 0s`.

### Credentials

* Obtain Google credentials
  * [Create an Oauth Client ID credential](https://console.cloud.google.com/apis/credentials)
    * You may need to create a project, or use a dev project you have access to. If the project doesn't have OAuth consent screen info, you'll need to add that as well.
      * Choose "internal" user type, give it a name similar to the project name, and add your contact email
    * Choose "Web application" as the client ID type
    * Set Authorized redirect URIs to `http://localhost:8080/oauth/google`
  * Save credentials to `google.config.json` (GCP Console > API & Services > Credentials > Download JSON)

* Obtain Zoom credentials
  * [Create an Oauth Application](https://marketplace.zoom.us/develop/create)
    * User-managed
    * No need to publish
    * Set name, descriptions, and contact information
    * Add the `recording:read` scope 
    * Set redirect URI to `http://127.0.0.1.ip.es.io:8080/oauth/zoom` and also add it to Whitelisted URLs
    * 
  * Save credentials to `zoom.config.json` with content:
    ```json
    {
      "id":             "your-id",
      "secret":         "your-secret",
      "oauth_redirect": "http://127.0.0.1.ip.es.io:8080/oauth/zoom"
    }
    ```
* [Optional] Obtain Slack credentials
  * [Create an App](https://api.slack.com/apps?new_app=1)
    * Add Permissions > Scopes > Bot Token Scopes > Add An Oauth Scope granting: `channels:read`, `chat:write`, `chat:write.public`
  * Save the Bot User OAuth Access Token (under OAuth & Permissions) to `slack.config.json` with content:
    ```json
    {
      "token":             "your-token"
    }
    ```

Once the credentials are in place, re-run `zat` and use the web server at http://localhost:8080/ to login to both Google and Zoom to create the `*.creds.json` files zat will use for the next run.

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

Where `google` is the folder ID to store recordings into, and `zoom` is the meeting id (hyphens or no hyphens, not spaces).

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

You'll likely get an "Access Not Configured" error for new projects. Follow the URL in the error to ensure the project is enabled for Google Drive API access, then wait a few minutes before retrying.

zat provides a web interface with similar functionality, eg [http://localhost:8080/google?q=name contains "Team weekly"](http://localhost:8080/google?q=name%20contains%20%27Team%20weekly%27).

#### Zoom

The zoom configuration is the meeting ID - the dashes are optional.

`cmd/zoom/listrecordings` can assist in tracking down meeting IDs like:

```
$ go build ./cmd/zoom/listrecordings
$ ./listrecordings -since 96h
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

#### Slack

The slack configuration is the ID of the channel where the message should be sent.

`cmd/slack/listchannels` can assist in tracking down channel IDs like:

```
$ go build ./cmd/slack/listchannels
$ ./listchannels
CAAAAAAAA general
CAAAAAAAB zat

One method for finding private channel IDs is to open Slack in a web browser and look at `$$('.p-channel_sidebar__static_list__item')` elements.
The application's bot user will need to be invited to the private channel to post messages there.
```

`cmd/slack/chat` can assist in verifying permissions are correct.

#### Scheduling

On macOS pre-10.15 (Catalina) and Linux, `cron` is sufficient, eg:

```
0 8,10,15,22 * * * zat -no-server -config-dir ~/path/to/zat/config/dir
```

On macOS 10.15+, new security restrictions make `cron` less attractive.

Instead use `launchd`.
A sample configuration is included under `contrib/`.
Load it with:
```
launchctl load contrib/zat.plist
```

If prompted the first time the job runs, grant `zat` access to the config directory.

## Also

* Zoom doesn't look back farther than 30 days when `-since` is > 30 days. - [#16](https://github.com/graphaelli/zat/issues/16)
