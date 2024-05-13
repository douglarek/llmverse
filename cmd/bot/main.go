package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/douglarek/llmverse/config"
	"github.com/douglarek/llmverse/internal/discordbot"
)

var configFile = flag.String("config-file", "config.json", "path to config file")
var slogLevel = new(slog.LevelVar)

func init() {
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel})
	slog.SetDefault(slog.New(h))
}

func main() {
	flag.Parse()

	settings, err := config.LoadSettings(*configFile)
	if err != nil {
		slog.Error("[main]: cannot load settings", "error", err)
		return

	}
	if settings.EnableDebug {
		slogLevel.Set(slog.LevelDebug)
	}

	bot, err := discordbot.New(settings)
	if err != nil {
		slog.Error("[main]: cannot create discord bot", "error", err)
		return
	}
	defer bot.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	slog.Info("[main]: bot is running, press Ctrl+C to exit")
	<-stop

	slog.Info("[main]: bot is gracefully shutting down")
}
