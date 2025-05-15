package bot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/morrisonbrett/SummerRateChecker/internal/config"
	"github.com/morrisonbrett/SummerRateChecker/internal/morpho"
	"github.com/morrisonbrett/SummerRateChecker/internal/storage"
	"github.com/morrisonbrett/SummerRateChecker/internal/types"
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

	// Add intent for message content
	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	// Add message handler
	session.AddHandler(bot.messageHandler)

	return bot, nil
}

func (b *Bot) Start() error {
	err := b.session.Open()
	if err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}
	b.logger.Info("Discord bot connected")
	return nil
}

func (b *Bot) Stop() error {
	return b.session.Close()
}

func (b *Bot) GetCheckTrigger() <-chan bool {
	return b.checkTrigger
}

func (b *Bot) messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check if it's a command (starts with !)
	if !strings.HasPrefix(m.Content, "!") {
		return
	}

	// Parse command and arguments, handling quoted strings
	content := m.Content[1:] // Remove the !
	command := ""
	var args []string

	// Find the command (first word)
	if spaceIndex := strings.Index(content, " "); spaceIndex != -1 {
		command = strings.ToLower(content[:spaceIndex])
		content = content[spaceIndex+1:]
	} else {
		command = strings.ToLower(content)
	}

	// Parse arguments, handling quoted strings
	inQuote := false
	var currentArg strings.Builder
	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '"':
			inQuote = !inQuote
		case ' ':
			if !inQuote {
				if currentArg.Len() > 0 {
					args = append(args, currentArg.String())
					currentArg.Reset()
				}
			} else {
				currentArg.WriteByte(content[i])
			}
		default:
			currentArg.WriteByte(content[i])
		}
	}
	if currentArg.Len() > 0 {
		args = append(args, currentArg.String())
	}

	b.logger.Infof("Command received: %s from %s (args: %v)", command, m.Author.Username, args)

	// Route command
	switch command {
	case "enroll":
		b.handleEnroll(s, m, args)
	case "unenroll":
		b.handleUnenroll(s, m, args)
	case "list":
		b.handleList(s, m, args)
	case "status":
		b.handleStatus(s, m, args)
	case "check":
		b.handleCheck(s, m, args)
	case "threshold":
		b.handleThreshold(s, m, args)
	case "interval":
		b.handleInterval(s, m, args)
	case "help":
		b.handleHelp(s, m, args)
	default:
		b.sendMessage(s, m.ChannelID, "Unknown command. Use `!help` for available commands.")
	}
}

func (b *Bot) handleEnroll(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	if len(args) < 3 {
		b.sendMessage(s, m.ChannelID, "Usage: `!enroll <summer.fi_url> <\"nickname\"> <threshold> [channel]`\n"+
			"Example: `!enroll https://pro.summer.fi/ethereum/morphoblue/borrow/WBTC-USDC/1234#overview \"My WBTC Vault\" 0.5 #rate-alerts`\n"+
			"Note: You must provide the full Summer.fi URL for your vault")
		return
	}

	// Validate that the first argument is a URL
	if !strings.HasPrefix(args[0], "http") {
		b.sendMessage(s, m.ChannelID, "Please provide the full Summer.fi URL for your vault.\n"+
			"Example: `!enroll https://pro.summer.fi/ethereum/morphoblue/borrow/WBTC-USDC/1234#overview \"My WBTC Vault\" 0.5 #rate-alerts`")
		return
	}

	urlInfo, err := morpho.ParseVaultURL(args[0])
	if err != nil {
		b.sendMessage(s, m.ChannelID, fmt.Sprintf("Invalid Summer.fi URL: %v\n"+
			"Please provide a valid URL in the format: https://pro.summer.fi/ethereum/morphoblue/borrow/MARKET-PAIR/VAULT-ID#overview", err))
		return
	}

	// Nickname should already be properly parsed with quotes
	nickname := strings.Trim(args[1], `"`)

	// Parse threshold and channel
	thresholdStr := args[2]
	channelID := m.ChannelID // Default to current channel

	// Check if we have a channel argument
	if len(args) > 3 {
		channelID = args[3]
	}

	// Clean up channel ID if it's a mention
	if strings.HasPrefix(channelID, "<#") && strings.HasSuffix(channelID, ">") {
		channelID = channelID[2 : len(channelID)-1]
	}

	// Parse the threshold
	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		b.sendMessage(s, m.ChannelID, fmt.Sprintf("Invalid threshold '%s'. Please provide a number between 0.1 and 100.0", thresholdStr))
		return
	}

	if threshold <= 0 || threshold > 100 {
		b.sendMessage(s, m.ChannelID, "Threshold must be between 0.1 and 100.0")
		return
	}

	vault := &types.VaultConfig{
		VaultID:          urlInfo.VaultID,
		Nickname:         nickname,
		ThresholdPercent: threshold,
		ChannelID:        channelID,
		MarketPair:       urlInfo.MarketPair,
	}

	err = b.storage.AddVault(vault)
	if err != nil {
		b.sendMessage(s, m.ChannelID, "Failed to enroll vault: "+err.Error())
		return
	}

	response := fmt.Sprintf(
		"✅ Successfully enrolled vault `%s` (\"%s\")\n"+
			"Market Pair: %s\n"+
			"Threshold: %.1f%%\n"+
			"Alerts will be sent to <#%s>",
		urlInfo.VaultID, nickname, urlInfo.MarketPair, threshold, channelID,
	)
	b.sendMessage(s, m.ChannelID, response)
}

func (b *Bot) handleUnenroll(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	if len(args) < 1 {
		b.sendMessage(s, m.ChannelID, "Usage: `!unenroll <vault_id>`")
		return
	}

	vaultID := args[0]
	vault, err := b.storage.GetVault(vaultID)
	if err != nil {
		b.sendMessage(s, m.ChannelID, "Error checking vault: "+err.Error())
		return
	}

	if vault == nil {
		b.sendMessage(s, m.ChannelID, fmt.Sprintf("❌ Vault `%s` not found", vaultID))
		return
	}

	err = b.storage.RemoveVault(vaultID)
	if err != nil {
		b.sendMessage(s, m.ChannelID, "Failed to unenroll vault: "+err.Error())
		return
	}

	b.sendMessage(s, m.ChannelID, fmt.Sprintf("✅ Unenrolled vault `%s`", vaultID))
}

func (b *Bot) handleList(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	vaults, err := b.storage.GetAllVaults()
	if err != nil {
		b.sendMessage(s, m.ChannelID, "Error retrieving vaults: "+err.Error())
		return
	}

	if len(vaults) == 0 {
		b.sendMessage(s, m.ChannelID, "No vaults enrolled")
		return
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

	b.sendMessage(s, m.ChannelID, response.String())
}

func (b *Bot) handleStatus(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	vaults, err := b.storage.GetAllVaults()
	if err != nil {
		b.sendMessage(s, m.ChannelID, "Error retrieving vaults: "+err.Error())
		return
	}

	if len(vaults) == 0 {
		b.sendMessage(s, m.ChannelID, "No vaults enrolled")
		return
	}

	lastRates := b.storage.GetAllLastRates()

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

	b.sendMessage(s, m.ChannelID, response.String())
}

func (b *Bot) handleCheck(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	// Trigger an immediate check
	select {
	case b.checkTrigger <- true:
		b.sendMessage(s, m.ChannelID, "🔄 Manual rate check triggered! Checking all vaults now...")
	default:
		b.sendMessage(s, m.ChannelID, "🔄 Manual check already in progress, please wait...")
	}
}

func (b *Bot) handleThreshold(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	if len(args) < 2 {
		b.sendMessage(s, m.ChannelID, "Usage: `!threshold <vault_id> <new_threshold>`")
		return
	}

	vaultID := args[0]
	newThreshold, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		b.sendMessage(s, m.ChannelID, "Invalid threshold. Please provide a number.")
		return
	}

	if newThreshold <= 0 || newThreshold > 100 {
		b.sendMessage(s, m.ChannelID, "Threshold must be between 0.1 and 100.0")
		return
	}

	vault, err := b.storage.GetVault(vaultID)
	if err != nil {
		b.sendMessage(s, m.ChannelID, "Error checking vault: "+err.Error())
		return
	}

	if vault == nil {
		b.sendMessage(s, m.ChannelID, fmt.Sprintf("❌ Vault `%s` not found", vaultID))
		return
	}

	vault.ThresholdPercent = newThreshold
	err = b.storage.AddVault(vault) // This updates the existing vault
	if err != nil {
		b.sendMessage(s, m.ChannelID, "Failed to update threshold: "+err.Error())
		return
	}

	b.sendMessage(s, m.ChannelID, fmt.Sprintf(
		"✅ Updated threshold for `%s` to %.1f%%",
		vaultID, newThreshold,
	))
}

func (b *Bot) handleInterval(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	response := fmt.Sprintf("Current check interval: %d minutes", b.config.Monitor.CheckIntervalMinutes)
	b.sendMessage(s, m.ChannelID, response)
}

func (b *Bot) handleHelp(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	help := `**SummerRateChecker Commands:**

🏦 **Vault Management:**
• !enroll <summer.fi_url> <"nickname"> <threshold> [channel] - Add a vault for monitoring
  - Example: ` + "`!enroll https://pro.summer.fi/ethereum/morphoblue/borrow/WBTC-USDC/1234#overview \"My WBTC Vault\" 0.5 #rate-alerts`" + `
  - Note: You must provide the full Summer.fi URL for your vault
• !unenroll <vault_id> - Remove a vault from monitoring
• !list - Show all enrolled vaults with their market pairs and rates

📊 **Monitoring:**
• !status - Show current rates for all vaults
• !check - Force an immediate rate check
• !threshold <vault_id> <new_threshold> - Update alert threshold
• !interval - Show current check interval

ℹ️ **General:**
• !help - Show this help message

**Notes:**
• Nicknames can contain spaces and must be enclosed in quotes
• Threshold is in percentage points (0.5 = alert on ±0.5% change)
• You must provide the full Summer.fi URL when enrolling a vault
• The URL format is: ` + "`https://pro.summer.fi/ethereum/morphoblue/borrow/MARKET-PAIR/VAULT-ID#overview`" + `
`
	b.sendMessage(s, m.ChannelID, help)
}

func (b *Bot) sendMessage(s *discordgo.Session, channelID, content string) {
	_, err := s.ChannelMessageSend(channelID, content)
	if err != nil {
		b.logger.Errorf("Failed to send message: %v", err)
	}
}
