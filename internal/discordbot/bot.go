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

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "llmverse",
		Description: "A top-level command for LLMVerse",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "models",
				Description: "subcommand to list all available models",
			},
		},
	},
}

var registeredCommands = make([]*discordgo.ApplicationCommand, len(commands))

type Bot struct {
	session *discordgo.Session
}

func (b *Bot) Close() error {
	for _, v := range registeredCommands {
		err := b.session.ApplicationCommandDelete(b.session.State.User.ID, "", v.ID)
		if err != nil {
			slog.Error("[main]: cannot delete command", "error", err)
		}
	}

	return b.session.Close()
}

func New(settings config.Settings) (*Bot, error) {
	session, err := discordgo.New("Bot " + settings.DiscordBotToken)
	if err != nil {
		return nil, err
	}

	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		slog.Info("[main]: bot is ready", "user", r.User.Username+"#"+r.User.Discriminator)
	})
	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.ApplicationCommandData().Name == "llmverse" {
			options := i.ApplicationCommandData().Options
			var content string

			switch options[0].Name {
			case "models":
				content = "Available models: " + settings.GetAvailableModels()
			default:
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: content,
				},
			})
		}
	})
	session.AddHandler(messageCreate(aicore.NewLLMAgent(settings)))
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

	err = session.Open()
	if err != nil {
		return nil, err
	}

	for i, v := range commands {
		cmd, err := session.ApplicationCommandCreate(session.State.User.ID, "", v)
		if err != nil {
			return nil, err
		}
		registeredCommands[i] = cmd
	}

	return &Bot{session: session}, nil
}

func messageCreate(m *aicore.LLMAgent) func(s *discordgo.Session, e *discordgo.MessageCreate) {
	return func(s *discordgo.Session, e *discordgo.MessageCreate) {
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
		slog.Debug("[main.messageHandler]: received message", "content", e.Content, "author", e.Author.Username, "channel", e.ChannelID, "guild", e.GuildID)
		rawConent := strings.TrimLeftFunc(regexp.MustCompile("<[^>]+>").ReplaceAllString(e.Content, ""), unicode.IsSpace)

		if rawConent == "$clear" {
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
