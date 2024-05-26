package discordbot

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/bwmarrin/discordgo"
	"github.com/douglarek/llmverse/aicore"
	"github.com/douglarek/llmverse/config"
)

type Bot struct {
	session *discordgo.Session
}

func (b *Bot) Close() error {

	return b.session.Close()
}

func New(settings config.Settings) (*Bot, error) {
	session, err := discordgo.New("Bot " + settings.DiscordBotToken)
	if err != nil {
		return nil, err
	}

	session.AddHandler(botReady)
	session.AddHandler(messageCreate(aicore.NewLLMAgent(settings)))
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

	err = session.Open()
	if err != nil {
		return nil, err
	}

	return &Bot{session: session}, nil
}

func botReady(s *discordgo.Session, r *discordgo.Ready) {
	slog.Info("[main]: bot is ready", "user", r.User.Username+"#"+r.User.Discriminator)
}

func messageCreate(agent *aicore.LLMAgent) func(s *discordgo.Session, e *discordgo.MessageCreate) {
	return func(s *discordgo.Session, e *discordgo.MessageCreate) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()

		if e.Author.ID == s.State.User.ID || e.MentionEveryone { // ignore this bot and disable @everyone
			return
		}

		var shouldReply bool
		for _, mention := range e.Mentions {
			if mention.ID == s.State.User.ID {
				shouldReply = true
				break
			}
		}

		if !shouldReply && e.GuildID != "" {
			return
		}

		s.MessageReactionAdd(e.ChannelID, e.ID, "💬")
		s.ChannelTyping(e.ChannelID)
		rawConent := strings.TrimLeftFunc(regexp.MustCompile("<[^>]+>").ReplaceAllString(e.Content, ""), unicode.IsSpace)

		if rawConent == "$clear" {
			agent.ClearHistory(ctx, e.Author.Username)
			s.ChannelMessageSendReply(e.ChannelID, "🤖 history cleared.", e.Reference())
			return
		}

		var modelName string
		if modelName = config.ParseModelName(rawConent); modelName == "" {
			if e.ReferencedMessage != nil {
				modelName = config.ParseModelName(e.ReferencedMessage.Content)
				if modelName != "" {
					rawConent = modelName + ": " + rawConent
				}
			}
		}

		var imageURLs []string
		var resp any
		var err error
		if len(e.Attachments) > 0 {
			for _, a := range e.Attachments {
				if strings.HasSuffix(a.Filename, ".png") ||
					strings.HasSuffix(a.Filename, ".jpg") ||
					strings.HasSuffix(a.Filename, ".jpeg") ||
					strings.HasSuffix(a.Filename, ".gif") ||
					strings.HasSuffix(a.Filename, ".webp") {
					imageURLs = append(imageURLs, a.URL)
				}
			}
			if len(imageURLs) == 0 {
				resp = "🤖 no image found. only png, jpg, jpeg, gif or webp supported"
			} else {
				resp, err = agent.Query(ctx, e.Author.Username, rawConent, imageURLs)
			}
		} else {
			resp, err = agent.Query(ctx, e.Author.Username, rawConent, nil)
		}

		if err != nil {
			s.ChannelMessageSendReply(e.ChannelID, "🤖 "+err.Error(), e.Reference())
			return
		}

		switch output := resp.(type) {
		case string:
			s.ChannelMessageSendReply(e.ChannelID, output, e.Reference())
		case <-chan string:
			var message string
			messageObj, _ := s.ChannelMessageSendReply(e.ChannelID, "✏️ ...", e.Reference())
			s.ChannelTyping(e.ChannelID)

			tk := time.NewTicker(1 * time.Second)
		L:
			for {
				select {
				case <-tk.C:
					s.ChannelTyping(e.ChannelID)
					umessage := []rune(message)
					if len(umessage) <= 2000 {
						s.ChannelMessageEdit(e.ChannelID, messageObj.ID, message)
						continue
					}

					s.ChannelMessageEdit(e.ChannelID, messageObj.ID, string(umessage[:2000]))
					message = modelName + ": ⏩ " + string(umessage[2000:])
					messageObj, _ = s.ChannelMessageSendReply(e.ChannelID, message, e.Reference())
				default:
					chunk, ok := <-output
					if !ok {
						time.Sleep(1 * time.Second) // discord 429 case
						umessage := []rune(message)
						if len(umessage) <= 2000 {
							s.ChannelMessageEdit(e.ChannelID, messageObj.ID, message)
							return
						}
						message = string(umessage[2000:])
						s.ChannelMessageSendReply(e.ChannelID, message, e.Reference())
						tk.Stop()
						break L
					}
					message += chunk
				}
			}
		}
	}
}
