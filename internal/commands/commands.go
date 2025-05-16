package commands

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/morrisonbrett/SummerRateChecker/internal/config"
	"github.com/morrisonbrett/SummerRateChecker/internal/morpho"
	"github.com/morrisonbrett/SummerRateChecker/internal/storage"
	"github.com/morrisonbrett/SummerRateChecker/internal/types"
	"go.uber.org/zap"
)

// Command represents a slash command
type Command struct {
	Name        string
	Description string
	Options     []*discordgo.ApplicationCommandOption
	Handler     func(s *discordgo.Session, i *discordgo.InteractionCreate, ctx *CommandContext) error
}

// CommandContext holds dependencies needed by command handlers
type CommandContext struct {
	Config  *config.Config
	Storage storage.Storage
	Logger  *zap.SugaredLogger
	Trigger chan bool
}

// All available commands
var Commands = []*discordgo.ApplicationCommand{
	{
		Name:        "enroll",
		Description: "Add a vault for monitoring",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "url",
				Description: "Full Summer.fi URL for your vault",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "nickname",
				Description: "Nickname for the vault",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionNumber,
				Name:        "threshold",
				Description: "Alert threshold (0.1-100.0)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionChannel,
				Name:        "channel",
				Description: "Channel to send alerts to (defaults to current channel)",
				Required:    false,
				ChannelTypes: []discordgo.ChannelType{
					discordgo.ChannelTypeGuildText,
				},
			},
		},
	},
	{
		Name:        "unenroll",
		Description: "Remove a vault from monitoring",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "vault_id",
				Description: "ID of the vault to remove",
				Required:    true,
			},
		},
	},
	{
		Name:        "list",
		Description: "Show all enrolled vaults with their market pairs and rates",
	},
	{
		Name:        "status",
		Description: "Show current rates for all vaults",
	},
	{
		Name:        "check",
		Description: "Force an immediate rate check",
	},
	{
		Name:        "threshold",
		Description: "Update alert threshold for a vault",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "vault_id",
				Description: "ID of the vault to update",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionNumber,
				Name:        "new_threshold",
				Description: "New threshold value (0.1-100.0)",
				Required:    true,
			},
		},
	},
	{
		Name:        "interval",
		Description: "Show current check interval",
	},
	{
		Name:        "help",
		Description: "Show help message with all available commands",
	},
}

// RegisterCommands registers all slash commands with Discord
func RegisterCommands(s *discordgo.Session, appID string, guildID string) error {
	// Log the app ID and guild ID we're using
	fmt.Printf("Registering commands for application ID: %s in guild: %s\n", appID, guildID)

	// First, clean up any global commands (these should never exist)
	fmt.Println("Checking for global commands...")
	globalCommands, err := s.ApplicationCommands(appID, "")
	if err != nil {
		return fmt.Errorf("failed to get global commands: %w", err)
	}

	if len(globalCommands) > 0 {
		fmt.Printf("Found %d global commands to remove\n", len(globalCommands))
		for _, cmd := range globalCommands {
			fmt.Printf("Removing global command: %s (ID: %s)\n", cmd.Name, cmd.ID)
			err := s.ApplicationCommandDelete(appID, "", cmd.ID)
			if err != nil {
				return fmt.Errorf("failed to delete global command %s: %w", cmd.Name, err)
			}
		}
	}

	// Get existing guild commands
	fmt.Printf("Checking guild commands for guild %s...\n", guildID)
	existingCommands, err := s.ApplicationCommands(appID, guildID)
	if err != nil {
		return fmt.Errorf("failed to get guild commands: %w", err)
	}

	// Create maps for quick lookup
	existingMap := make(map[string]*discordgo.ApplicationCommand)
	for _, cmd := range existingCommands {
		existingMap[cmd.Name] = cmd
	}

	// Track which commands we've processed
	processedCommands := make(map[string]bool)

	// Update or create commands as needed
	fmt.Println("Updating commands...")
	for _, newCmd := range Commands {
		processedCommands[newCmd.Name] = true
		existingCmd, exists := existingMap[newCmd.Name]

		if !exists {
			// Command doesn't exist, create it
			fmt.Printf("Creating new command: %s\n", newCmd.Name)
			_, err := s.ApplicationCommandCreate(appID, guildID, newCmd)
			if err != nil {
				return fmt.Errorf("failed to create command %s: %w", newCmd.Name, err)
			}
			continue
		}

		// Check if command needs updating by comparing relevant fields
		if needsUpdate(existingCmd, newCmd) {
			fmt.Printf("Updating command: %s\n", newCmd.Name)
			_, err := s.ApplicationCommandEdit(appID, guildID, existingCmd.ID, newCmd)
			if err != nil {
				return fmt.Errorf("failed to update command %s: %w", newCmd.Name, err)
			}
		} else {
			fmt.Printf("Command unchanged: %s\n", newCmd.Name)
		}
	}

	// Remove any commands that no longer exist in our Commands list
	for name, cmd := range existingMap {
		if !processedCommands[name] {
			fmt.Printf("Removing obsolete command: %s\n", name)
			err := s.ApplicationCommandDelete(appID, guildID, cmd.ID)
			if err != nil {
				return fmt.Errorf("failed to delete obsolete command %s: %w", name, err)
			}
		}
	}

	fmt.Println("Command registration complete")
	return nil
}

// needsUpdate checks if a command needs to be updated by comparing relevant fields
func needsUpdate(existing, new *discordgo.ApplicationCommand) bool {
	// Compare basic fields
	if existing.Description != new.Description {
		return true
	}

	// Compare options
	if len(existing.Options) != len(new.Options) {
		return true
	}

	// Create maps for option comparison
	existingOpts := make(map[string]*discordgo.ApplicationCommandOption)
	for _, opt := range existing.Options {
		existingOpts[opt.Name] = opt
	}

	// Compare each option
	for _, newOpt := range new.Options {
		existingOpt, exists := existingOpts[newOpt.Name]
		if !exists {
			return true
		}

		// Compare option properties
		if existingOpt.Type != newOpt.Type ||
			existingOpt.Description != newOpt.Description ||
			existingOpt.Required != newOpt.Required {
			return true
		}

		// Compare channel types if present
		if len(existingOpt.ChannelTypes) != len(newOpt.ChannelTypes) {
			return true
		}
		for i, ct := range existingOpt.ChannelTypes {
			if i >= len(newOpt.ChannelTypes) || ct != newOpt.ChannelTypes[i] {
				return true
			}
		}
	}

	return false
}

// HandleCommand handles a slash command interaction
func HandleCommand(s *discordgo.Session, i *discordgo.InteractionCreate, ctx *CommandContext) {
	// Defer the response in case the handler takes time
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	var err error
	switch i.ApplicationCommandData().Name {
	case "enroll":
		err = handleEnroll(s, i, ctx)
	case "unenroll":
		err = handleUnenroll(s, i, ctx)
	case "list":
		err = handleList(s, i, ctx)
	case "status":
		err = handleStatus(s, i, ctx)
	case "check":
		err = handleCheck(s, i, ctx)
	case "threshold":
		err = handleThreshold(s, i, ctx)
	case "interval":
		err = handleInterval(s, i, ctx)
	case "help":
		err = handleHelp(s, i, ctx)
	default:
		err = fmt.Errorf("unknown command: %s", i.ApplicationCommandData().Name)
	}

	if err != nil {
		// Send error response
		errMsg := err.Error()
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &errMsg,
		})
		return
	}
}

// Command handlers
func handleEnroll(s *discordgo.Session, i *discordgo.InteractionCreate, ctx *CommandContext) error {
	options := i.ApplicationCommandData().Options
	url := options[0].StringValue()
	nickname := options[1].StringValue()
	threshold := options[2].FloatValue()

	// Validate threshold
	if threshold < 0.1 || threshold > 100.0 {
		return fmt.Errorf("threshold must be between 0.1 and 100.0")
	}

	// Get channel if provided, otherwise use current channel
	channelID := i.ChannelID
	if len(options) > 3 {
		channelID = options[3].ChannelValue(s).ID
	}

	// Create a webhook for the channel
	webhook, err := s.WebhookCreate(channelID, "SummerRateChecker", "")
	if err != nil {
		return fmt.Errorf("failed to create webhook for channel: %w", err)
	}

	urlInfo, err := morpho.ParseVaultURL(url)
	if err != nil {
		// Clean up webhook if URL parsing fails
		s.WebhookDelete(webhook.ID)
		return fmt.Errorf("invalid Summer.fi URL: %v", err)
	}

	vault := &types.VaultConfig{
		VaultID:          urlInfo.VaultID,
		Nickname:         nickname,
		ThresholdPercent: threshold,
		ChannelID:        channelID,
		WebhookURL:       fmt.Sprintf("https://discord.com/api/webhooks/%s/%s", webhook.ID, webhook.Token),
		MarketPair:       urlInfo.MarketPair,
	}

	err = ctx.Storage.AddVault(vault)
	if err != nil {
		// Clean up webhook if storage fails
		s.WebhookDelete(webhook.ID)
		return fmt.Errorf("failed to enroll vault: %w", err)
	}

	response := fmt.Sprintf(
		"✅ Successfully enrolled vault `%s` (\"%s\")\n"+
			"Market Pair: %s\n"+
			"Threshold: %.1f%%\n"+
			"Alerts will be sent to <#%s>",
		urlInfo.VaultID, nickname, urlInfo.MarketPair, threshold, channelID,
	)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &response,
	})
	return nil
}

func handleUnenroll(s *discordgo.Session, i *discordgo.InteractionCreate, ctx *CommandContext) error {
	vaultID := i.ApplicationCommandData().Options[0].StringValue()

	vault, err := ctx.Storage.GetVault(vaultID)
	if err != nil {
		return fmt.Errorf("error checking vault: %w", err)
	}

	if vault == nil {
		return fmt.Errorf("vault `%s` not found", vaultID)
	}

	// Delete the webhook if it exists
	if vault.WebhookURL != "" {
		// Extract webhook ID from URL
		parts := strings.Split(vault.WebhookURL, "/")
		if len(parts) >= 2 {
			webhookID := parts[len(parts)-2]
			if err := s.WebhookDelete(webhookID); err != nil {
				ctx.Logger.Warnf("Failed to delete webhook for vault %s: %v", vaultID, err)
			}
		}
	}

	err = ctx.Storage.RemoveVault(vaultID)
	if err != nil {
		return fmt.Errorf("failed to unenroll vault: %w", err)
	}

	response := fmt.Sprintf("✅ Unenrolled vault `%s`", vaultID)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &response,
	})
	return nil
}

func handleList(s *discordgo.Session, i *discordgo.InteractionCreate, ctx *CommandContext) error {
	vaults, err := ctx.Storage.GetAllVaults()
	if err != nil {
		return fmt.Errorf("error retrieving vaults: %w", err)
	}

	if len(vaults) == 0 {
		response := "No vaults enrolled"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &response,
		})
		return nil
	}

	var response strings.Builder
	response.WriteString("**Enrolled Vaults:**\n")
	for _, vault := range vaults {
		marketPair := vault.MarketPair
		if marketPair == "" {
			marketPair = "Unknown"
		}
		response.WriteString(fmt.Sprintf(
			"`%s` - \"%s\" (%s) - %.1f%% threshold → <#%s>\n",
			vault.VaultID, vault.Nickname, marketPair, vault.ThresholdPercent, vault.ChannelID,
		))
	}

	content := response.String()
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
	return nil
}

func handleStatus(s *discordgo.Session, i *discordgo.InteractionCreate, ctx *CommandContext) error {
	vaults, err := ctx.Storage.GetAllVaults()
	if err != nil {
		return fmt.Errorf("error retrieving vaults: %w", err)
	}

	if len(vaults) == 0 {
		response := "No vaults enrolled"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &response,
		})
		return nil
	}

	lastRates := ctx.Storage.GetAllLastRates()

	var response strings.Builder
	response.WriteString("**Current Status:**\n")
	for _, vault := range vaults {
		marketPair := vault.MarketPair
		if marketPair == "" {
			marketPair = "Unknown"
		}
		if rate, exists := lastRates[vault.VaultID]; exists {
			response.WriteString(fmt.Sprintf(
				"`%s` - \"%s\" (%s): %.2f%%\n",
				vault.VaultID, vault.Nickname, marketPair, rate,
			))
		} else {
			response.WriteString(fmt.Sprintf(
				"`%s` - \"%s\" (%s): Not checked yet\n",
				vault.VaultID, vault.Nickname, marketPair,
			))
		}
	}

	content := response.String()
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
	return nil
}

func handleCheck(s *discordgo.Session, i *discordgo.InteractionCreate, ctx *CommandContext) error {
	select {
	case ctx.Trigger <- true:
		response := "🔄 Manual rate check triggered! Checking all vaults now..."
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &response,
		})
	default:
		response := "🔄 Manual check already in progress, please wait..."
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &response,
		})
	}
	return nil
}

func handleThreshold(s *discordgo.Session, i *discordgo.InteractionCreate, ctx *CommandContext) error {
	options := i.ApplicationCommandData().Options
	vaultID := options[0].StringValue()
	newThreshold := options[1].FloatValue()

	// Validate threshold
	if newThreshold < 0.1 || newThreshold > 100.0 {
		return fmt.Errorf("threshold must be between 0.1 and 100.0")
	}

	vault, err := ctx.Storage.GetVault(vaultID)
	if err != nil {
		return fmt.Errorf("error checking vault: %w", err)
	}

	if vault == nil {
		return fmt.Errorf("vault `%s` not found", vaultID)
	}

	vault.ThresholdPercent = newThreshold
	err = ctx.Storage.AddVault(vault) // This updates the existing vault
	if err != nil {
		return fmt.Errorf("failed to update threshold: %w", err)
	}

	response := fmt.Sprintf(
		"✅ Updated threshold for `%s` to %.1f%%",
		vaultID, newThreshold,
	)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &response,
	})
	return nil
}

func handleInterval(s *discordgo.Session, i *discordgo.InteractionCreate, ctx *CommandContext) error {
	response := fmt.Sprintf("Current check interval: %d minutes", ctx.Config.Monitor.CheckIntervalMinutes)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &response,
	})
	return nil
}

func handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate, ctx *CommandContext) error {
	help := `**SummerRateChecker Commands:**

🏦 **Vault Management:**
• /enroll - Add a vault for monitoring
  - Required: URL, nickname, threshold
  - Optional: channel
  - Example: [Command Format] /enroll url:<summer-fi-url> nickname:My WBTC Vault threshold:0.5
• /unenroll - Remove a vault from monitoring
• /list - Show all enrolled vaults
• /threshold - Update alert threshold

📊 **Monitoring:**
• /status - Show current rates for all vaults
• /check - Force an immediate rate check
• /interval - Show current check interval

ℹ️ **General:**
• /help - Show this help message

**Notes:**
• Threshold is in percentage points (0.5 = alert on ±0.5% change)
• You must provide the full Summer.fi URL when enrolling a vault
• The URL format is: [URL Format] <summer-fi-url>
  Example: [Example URL] <https://pro.summer.fi/ethereum/morphoblue/borrow/WBTC-USDC/1234#overview>

Type "/" to see all available commands with their descriptions and options.`

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &help,
	})
	return nil
}

func ptr[T any](v T) *T {
	return &v
}
