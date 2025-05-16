package bot

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/morrisonbrett/SummerRateChecker/internal/commands"
	"github.com/morrisonbrett/SummerRateChecker/internal/config"
	"github.com/morrisonbrett/SummerRateChecker/internal/storage"
	"go.uber.org/zap"
)

type Bot struct {
	session      *discordgo.Session
	config       *config.Config
	storage      storage.Storage
	logger       *zap.SugaredLogger
	checkTrigger chan bool // Channel to trigger manual checks
}

func New(cfg *config.Config, store storage.Storage, logger *zap.SugaredLogger) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}

	bot := &Bot{
		session:      session,
		config:       cfg,
		storage:      store,
		logger:       logger,
		checkTrigger: make(chan bool, 1), // Buffered channel for manual triggers
	}

	// Add required intents for slash commands and interactions
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent |
		discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessageReactions

	// Add handlers
	session.AddHandler(bot.interactionHandler)
	session.AddHandler(bot.readyHandler) // Add ready handler

	return bot, nil
}

func (b *Bot) Start() error {
	// Open the session first
	err := b.session.Open()
	if err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}

	// Wait a moment for the session to be ready
	time.Sleep(2 * time.Second)

	// Get the first guild ID (since we're only in one server)
	var guildID string
	if len(b.session.State.Guilds) > 0 {
		guildID = b.session.State.Guilds[0].ID
		b.logger.Infof("Registering commands for guild: %s", guildID)
	} else {
		return fmt.Errorf("bot is not in any guilds")
	}

	// Now register slash commands after session is open
	err = commands.RegisterCommands(b.session, b.session.State.User.ID, guildID)
	if err != nil {
		b.session.Close() // Clean up session if command registration fails
		return fmt.Errorf("failed to register commands: %w", err)
	}

	b.logger.Info("Discord bot connected and commands registered")
	return nil
}

func (b *Bot) Stop() error {
	return b.session.Close()
}

func (b *Bot) GetCheckTrigger() <-chan bool {
	return b.checkTrigger
}

func (b *Bot) interactionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Only handle slash commands
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	// Create command context
	ctx := &commands.CommandContext{
		Config:  b.config,
		Storage: b.storage,
		Logger:  b.logger,
		Trigger: b.checkTrigger,
	}

	// Handle the command
	commands.HandleCommand(s, i, ctx)
}

func (b *Bot) readyHandler(s *discordgo.Session, r *discordgo.Ready) {
	b.logger.Infof("Bot is ready! Logged in as %s#%s (ID: %s)", r.User.Username, r.User.Discriminator, r.User.ID)
	b.logger.Infof("Connected to %d guilds:", len(r.Guilds))
	for _, guild := range r.Guilds {
		b.logger.Infof("  - %s (ID: %s)", guild.Name, guild.ID)
	}

	// Verify bot permissions
	for _, guild := range r.Guilds {
		member, err := s.GuildMember(guild.ID, r.User.ID)
		if err != nil {
			b.logger.Errorf("Failed to get member info for guild %s: %v", guild.Name, err)
			continue
		}

		// Get guild roles
		roles, err := s.GuildRoles(guild.ID)
		if err != nil {
			b.logger.Errorf("Failed to get roles for guild %s: %v", guild.Name, err)
			continue
		}

		// Calculate bot permissions
		var permissions int64
		for _, roleID := range member.Roles {
			for _, role := range roles {
				if role.ID == roleID {
					permissions |= role.Permissions
				}
			}
		}

		// Check required permissions
		required := int64(discordgo.PermissionSendMessages |
			discordgo.PermissionUseSlashCommands |
			discordgo.PermissionReadMessageHistory |
			discordgo.PermissionAddReactions)

		missing := required &^ permissions
		if missing != 0 {
			b.logger.Warnf("Bot is missing permissions in guild %s: %v", guild.Name, missing)
		} else {
			b.logger.Infof("Bot has all required permissions in guild %s", guild.Name)
		}
	}
}

func (b *Bot) sendMessage(s *discordgo.Session, channelID, content string) {
	_, err := s.ChannelMessageSend(channelID, content)
	if err != nil {
		b.logger.Errorf("Failed to send message: %v", err)
	}
}
