package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
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
		slog.Error("[main]: cannot load env file", "error", err)
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

	slog.Info("[main]: starting bot")
	if err := s.Connect(context.TODO()); err != nil {
		slog.Error("[main]: cannot connect", "error", err)
	}
}

func messageHandler(s *state.State, m *aicore.LLMAgent) interface{} {
	return func(e *gateway.MessageCreateEvent) {
		if e.Author.Bot || e.MentionEveryone { // ignore this bot and disable @everyone
			return
		}

		var shouldReply bool
		for _, mention := range e.Mentions {
			if mention.ID == s.Ready().User.ID {
				shouldReply = true
				break
			}
		}

		shouldReply = shouldReply || !e.GuildID.IsValid() // direct message
		if !shouldReply {
			return
		}

		s.React(e.ChannelID, e.ID, "💬")
		s.Typing(e.ChannelID)
		slog.Debug("[main.messageHandler]: received message", "content", e.Content, "author", e.Author.Username, "channel", e.ChannelID, "guild", e.GuildID)
		rawConent := strings.TrimLeftFunc(regexp.MustCompile("<[^>]+>").ReplaceAllString(e.Content, ""), unicode.IsSpace)

		if rawConent == "$clear" {
			m.ClearHistory(s.Context(), e.Author.Username)
			s.SendMessageReply(e.ChannelID, "🤖 history cleared.", e.ID)
			return
		}

		var imageURLs []string
		var resp any
		var err error
		if len(e.Attachments) > 0 {
			for _, a := range e.Attachments {
				if strings.HasSuffix(a.Filename, ".png") || strings.HasSuffix(a.Filename, ".jpg") || strings.HasSuffix(a.Filename, ".jpeg") || strings.HasSuffix(a.Filename, ".gif") || strings.HasSuffix(a.Filename, ".webp") {
					imageURLs = append(imageURLs, a.URL)
				}
			}
			if len(imageURLs) == 0 {
				resp = "🤖 no image found. only png, jpg, jpeg, gif or webp supported"
			} else {
				resp, err = m.Query(context.Background(), e.Author.Username, rawConent, imageURLs)
			}
		} else {
			resp, err = m.Query(context.Background(), e.Author.Username, rawConent, nil)
		}

		if err != nil {
			if _, err := s.SendMessageReply(e.ChannelID, fmt.Sprintf("An error occurred: %v", err), e.ID); err != nil {
				slog.Error("[main.messageHandler]: cannot send message", "error", err)
			}
			return
		}

		switch output := resp.(type) {
		case string:
			if _, err := s.SendMessageReply(e.ChannelID, output, e.ID); err != nil {
				slog.Error("[main.messageHandler]: cannot send message", "error", err)
			}
		case <-chan string:
			var message string
			var mID discord.MessageID
			m, err := s.SendMessageReply(e.ChannelID, "✏️ ...", e.ID)
			if err != nil {
				slog.Error("[main.messageHandler]: cannot send message", "error", err)
				return
			}
			mID = m.ID
			s.Typing(e.ChannelID)

			tk := time.NewTicker(1 * time.Second)
		L:
			for {
				select {
				case <-tk.C:
					if len(message) > 2000 {
						continue
					}
					if _, err := s.EditMessage(e.ChannelID, mID, message+" ✏️ ..."); err != nil {
						slog.Error("[main.messageHandler]: cannot edit message", "error", err)
						return
					}
				default:
					chunk, ok := <-output
					if !ok {
						time.Sleep(1 * time.Second) // discord 429 case
						if len(message) > 2000 {
							for chunk := range output {
								message += chunk
							}
							if _, err := s.EditMessageComplex(e.ChannelID, mID, api.EditMessageData{
								Content: option.NewNullableString(message[:2000]),
								Files:   []sendpart.File{{Name: "message.md", Reader: strings.NewReader(message)}},
							}); err != nil {
								slog.Error("[main.messageHandler]: cannot edit message", "error", err)
							}
							return
						}
						if _, err := s.EditMessage(e.ChannelID, mID, message); err != nil {
							slog.Error("[main.messageHandler]: cannot edit message", "error", err)
							return
						}

						tk.Stop()
						break L
					}
					message += chunk
				}
			}
		}
	}
}
