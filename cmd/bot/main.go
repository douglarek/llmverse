package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/sendpart"
	"github.com/douglarek/llmverse/aicore"
	"github.com/douglarek/llmverse/config"
	"github.com/joho/godotenv"
)

var envFile = flag.String("env-file", ".env", "path to env file")
var slogLevel = new(slog.LevelVar)

func init() {
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel})
	slog.SetDefault(slog.New(h))
}

func main() {
	flag.Parse()

	if err := godotenv.Load(*envFile); err != nil {
		slog.Error("cannot load env file", "error", err)
		return
	}

	settings := config.Load()
	if settings.EnableDebug {
		slogLevel.Set(slog.LevelDebug)
	}

	s := state.New("Bot " + settings.DiscordBotToken)
	s.AddIntents(gateway.IntentGuilds)
	s.AddIntents(gateway.IntentDirectMessages) // DMs
	s.AddIntents(gateway.IntentGuildMessages)  // when @mentioned
	s.AddHandler(messageHandler(s, aicore.NewLLMAgent(settings)))

	if err := s.Connect(context.TODO()); err != nil {
		slog.Error("cannot connect", "error", err)
	}
}

func messageHandler(s *state.State, m *aicore.LLMAgent) interface{} {
	return func(e *gateway.MessageCreateEvent) {
		if !e.Author.Bot {
			s.React(e.ChannelID, e.ID, "💬")
			s.Typing(e.ChannelID)
			slog.Debug("received message", "content", e.Content, "author", e.Author.Username, "channel", e.ChannelID, "guild", e.GuildID)
			rawConent := strings.TrimLeftFunc(regexp.MustCompile("<[^>]+>").ReplaceAllString(e.Content, ""), unicode.IsSpace)
			resp, err := m.Query(context.Background(), e.Author.Username, rawConent)
			if err != nil {
				if _, err := s.SendMessageReply(e.ChannelID, fmt.Sprintf("An error occurred: %v", err), e.ID); err != nil {
					slog.Error("cannot send message", "error", err)
				}
			} else {
				if len(resp) > 2000 {
					if _, err := s.SendMessageComplex(e.ChannelID, api.SendMessageData{
						Content:   resp[:2000],
						Reference: &discord.MessageReference{MessageID: e.ID},
						Files:     []sendpart.File{{Name: "message.md", Reader: strings.NewReader(resp)}},
					}); err != nil {
						slog.Error("cannot send message", "error", err)
					}
					return
				}
				if _, err := s.SendMessageReply(e.ChannelID, resp, e.ID); err != nil {
					slog.Error("cannot send message", "error", err)
				}
			}
		}
	}
}
