package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"

	slackapi "github.com/slack-go/slack"

	"github.com/graphaelli/zat/cmd"
	"github.com/graphaelli/zat/slack"
)

func main() {
	cfgDir := cmd.FlagConfigDir()
	flag.Parse()

	logger := log.New(os.Stderr, "", cmd.LogFmt)
	api, _ := slack.NewClientFromEnvOrFile(logger, path.Join(*cfgDir, cmd.SlackConfigPath), slackapi.OptionDebug(true))
	if api == nil {
		logger.Fatal("failed to create slack api client")
	}

	next := ""
	for i := 0; i == 0 || next != ""; i++ {
		channels, nextCursor, err := api.GetConversations(
			&slackapi.GetConversationsParameters{
				Cursor:          next,
				ExcludeArchived: "true",
				Limit: 100,
			},
		)
		if err != nil {
			panic(err)
		}
		for _, channel := range channels {
			fmt.Printf("%s %s\n", channel.ID, channel.NameNormalized)
		}
		next = nextCursor
	}
}
