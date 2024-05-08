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

	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/douglarek/llmverse/aicore"
	"github.com/douglarek/llmverse/config"
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
			messageObj, err := s.SendMessageReply(e.ChannelID, "✏️ ...", e.ID)
			if err != nil {
				slog.Error("[main.messageHandler]: cannot send message", "error", err)
				return
			}
			s.Typing(e.ChannelID)

			tk := time.NewTicker(1 * time.Second)
		L:
			for {
				select {
				case <-tk.C:
					s.Typing(e.ChannelID)
					umessage := []rune(message)
					if len(umessage) <= 2000 {
						if _, err := s.EditMessage(e.ChannelID, messageObj.ID, message); err != nil {
							slog.Error("[main.messageHandler]: cannot edit message", "error", err)
							return
						}
						continue
					}

					if _, err := s.EditMessage(e.ChannelID, messageObj.ID, string(umessage[:2000])); err != nil {
						slog.Error("[main.messageHandler]: cannot edit message", "error", err)
						return
					}
					message = "⏩ " + string(umessage[2000:])
					messageObj, err = s.SendMessageReply(e.ChannelID, message, messageObj.ID)
					if err != nil {
						slog.Error("[main.messageHandler]: cannot send message", "error", err)
						return
					}
				default:
					chunk, ok := <-output
					if !ok {
						time.Sleep(1 * time.Second) // discord 429 case
						umessage := []rune(message)
						if len(umessage) <= 2000 {
							if _, err := s.EditMessage(e.ChannelID, messageObj.ID, message); err != nil {
								slog.Error("[main.messageHandler]: cannot edit message", "error", err)
								return
							}
							return
						}
						message = string(umessage[2000:])
						if _, err = s.SendMessageReply(e.ChannelID, message, messageObj.ID); err != nil {
							slog.Error("[main.messageHandler]: cannot send message", "error", err)
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
