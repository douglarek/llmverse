package bot

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/bwmarrin/discordgo"
	"github.com/douglarek/llmverse/aicore"
	"github.com/douglarek/llmverse/config"
)

type Discord struct {
	session *discordgo.Session
}

func (b *Discord) Close() error {
	return b.session.Close()
}

func NewDiscord(settings config.Settings) (*Discord, error) {
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

	return &Discord{session: session}, nil
}

func botReady(s *discordgo.Session, r *discordgo.Ready) {
	slog.Info("[main]: bot is ready", "user", r.User.Username+"#"+r.User.Discriminator)
}

func combineModelWithMessage(modelName, message string) string {
	return modelName + ": " + message
}

func combineModelWithErrMessage(modelName, message string) string {
	return modelName + ": 🤖 " + message
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

		rawConent := strings.TrimLeftFunc(regexp.MustCompile("<[^>]+>").ReplaceAllString(e.Content, ""), unicode.IsSpace)

		if rawConent == "$clear" {
			s.MessageReactionAdd(e.ChannelID, e.ID, "💬")
			agent.ClearHistory(ctx, e.Author.Username)
			s.ChannelMessageSendReply(e.ChannelID, "🤖 history cleared.", e.Reference())
			return
		} else if rawConent == "$models" {
			s.MessageReactionAdd(e.ChannelID, e.ID, "💬")
			resp := fmt.Sprintf("🤖 available models: %s. begin your question with `model: `", agent.AvailableModelNames())
			s.ChannelMessageSendReply(e.ChannelID, resp, e.Reference())
			return
		}

		var modelName string
		if modelName = agent.ParseModelName(rawConent); modelName == "" {
			if e.ReferencedMessage == nil {
				return
			}
			if modelName = agent.ParseModelName(e.ReferencedMessage.Content); modelName == "" {
				return
			}
		}

		s.MessageReactionAdd(e.ChannelID, e.ID, "💬")
		s.ChannelTyping(e.ChannelID)

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
				resp = "no image found. only png, jpg, jpeg, gif or webp supported"
			} else {
				resp, err = agent.Query(ctx, modelName, e.Author.Username, rawConent, imageURLs)
			}
		} else {
			resp, err = agent.Query(ctx, modelName, e.Author.Username, rawConent, nil)
		}

		if err != nil {
			s.ChannelMessageSendReply(e.ChannelID, combineModelWithErrMessage(modelName, err.Error()), e.Reference())
			return
		}

		switch output := resp.(type) {
		case string:
			s.ChannelMessageSendReply(e.ChannelID, combineModelWithErrMessage(modelName, output), e.Reference())
		case <-chan string:
			message := combineModelWithMessage(modelName, "")
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
					message = combineModelWithMessage(modelName, "⏩ ") + string(umessage[2000:])
					messageObj, _ = s.ChannelMessageSendReply(e.ChannelID, message, e.Reference())
				case chunk, ok := <-output:
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
