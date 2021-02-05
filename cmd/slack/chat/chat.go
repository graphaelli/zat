package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	slackapi "github.com/slack-go/slack"

	"github.com/graphaelli/zat/cmd"
	"github.com/graphaelli/zat/slack"
)

func main() {
	cfgDir := cmd.FlagConfigDir()
	noEscape := flag.Bool("n", false, "don't escape message text")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: %s channel [text ...]\n", os.Args[0])
		os.Exit(1)
	}
	channel := flag.Arg(0)
	text := "Hello, zat"
	if flag.NArg() > 1 {
		text = strings.Join(flag.Args()[1:], " ")
	}

	logger := log.New(os.Stderr, "", cmd.LogFmt)
	api, _ := slack.NewClientFromEnvOrFile(logger, path.Join(*cfgDir, cmd.SlackConfigPath), slackapi.OptionDebug(true))
	if api == nil {
		logger.Fatal("failed to create slack api client")
	}

	channel, ts, text, err := api.SendMessage(channel, slackapi.MsgOptionText(text, !*noEscape))
	if err != nil {
		panic(err)
	}
	fmt.Println(channel, ts, text)
}
