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

	channels, _, err := api.GetConversations(&slackapi.GetConversationsParameters{})
	if err != nil {
		panic(err)
	}
	for i, channel := range channels {
		fmt.Printf("%d: %+v\n", i, channel)
	}
}
