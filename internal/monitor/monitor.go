package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/morrisonbrett/SummerRateChecker/internal/config"
	"github.com/morrisonbrett/SummerRateChecker/internal/morpho"
	"github.com/morrisonbrett/SummerRateChecker/internal/storage"
	"github.com/morrisonbrett/SummerRateChecker/internal/types"
	"go.uber.org/zap"
)

type Monitor struct {
	config       *config.Config
	storage      storage.Storage
	morphoClient *morpho.Client
	httpClient   *http.Client
	logger       *zap.SugaredLogger
	checkTrigger <-chan bool
}

func New(cfg *config.Config, store storage.Storage, logger *zap.SugaredLogger) *Monitor {
	return &Monitor{
		config:       cfg,
		storage:      store,
		morphoClient: morpho.NewClient(cfg.Morpho.APIURL, logger),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		logger:       logger,
	}
}

func (m *Monitor) SetCheckTrigger(trigger <-chan bool) {
	m.checkTrigger = trigger
}

func (m *Monitor) CheckOnce() {
	m.checkAllVaults()
}

func (m *Monitor) Start() {
	ticker := time.NewTicker(time.Duration(m.config.Monitor.CheckIntervalMinutes) * time.Minute)
	defer ticker.Stop()

	m.logger.Infof("Starting rate monitor with %d minute intervals", m.config.Monitor.CheckIntervalMinutes)

	// Run initial check
	m.checkAllVaults()

	// Run periodic checks and listen for manual triggers
	for {
		select {
		case <-ticker.C:
			m.checkAllVaults()
		case <-m.checkTrigger:
			m.logger.Info("Manual check triggered")
			m.checkAllVaults()
		}
	}
}

func (m *Monitor) checkAllVaults() {
	m.checkRates(context.Background())
}

func (m *Monitor) checkRates(ctx context.Context) error {
	m.logger.Info("Checking rates for all vaults")

	// Get all vaults
	vaults, err := m.storage.GetAllVaults()
	if err != nil {
		return fmt.Errorf("failed to get vaults: %w", err)
	}

	if len(vaults) == 0 {
		m.logger.Info("No vaults to check")
		return nil
	}

	m.logger.Infof("Checking %d vaults", len(vaults))

	// Get current rates for all vaults
	marketData, err := m.morphoClient.GetMultipleMarkets(ctx, vaults)
	if err != nil {
		return fmt.Errorf("failed to get market data: %w", err)
	}

	// Process each vault's rate and build embeds
	var embeds []types.DiscordEmbed
	for _, data := range marketData {
		// Find the vault config using the vault ID
		var vaultConfig *types.VaultConfig
		for _, v := range vaults {
			if v.VaultID == data.VaultID {
				vaultConfig = v
				break
			}
		}

		if vaultConfig == nil {
			m.logger.Warnf("No vault config found for vault ID %s", data.VaultID)
			continue
		}

		// Get the last known rate
		lastRate, exists := m.storage.GetLastRate(vaultConfig.VaultID)
		if !exists {
			m.logger.Infof("First rate check for vault %s: %.4f%%", vaultConfig.Nickname, data.BorrowRate)
			if err := m.storage.UpdateLastRate(vaultConfig.VaultID, data.BorrowRate); err != nil {
				m.logger.Errorf("Failed to update last rate for %s: %v", vaultConfig.VaultID, err)
			}
			// Create embed for first check
			embed := types.DiscordEmbed{
				Title:       fmt.Sprintf("Rate Status: %s", vaultConfig.Nickname),
				Description: fmt.Sprintf("First rate check for %s", vaultConfig.Nickname),
				Color:       0x808080, // Gray for first check
				Fields: []types.DiscordEmbedField{
					{
						Name:   fmt.Sprintf("**Current Rate:** %.2f%%", data.BorrowRate),
						Value:  " ",
						Inline: false,
					},
					{
						Name:   "Market Pair",
						Value:  vaultConfig.MarketPair,
						Inline: true,
					},
				},
				Timestamp: time.Now().Format(time.RFC3339),
				Footer: &types.DiscordEmbedFooter{
					Text: "SummerRateChecker",
				},
			}
			embeds = append(embeds, embed)
			continue
		}

		// Calculate rate change
		rateChange := data.BorrowRate - lastRate
		rateChangePercent := math.Abs((rateChange / lastRate) * 100)

		// Only create status embed if there's an actual change
		if rateChange != 0 {
			// Create embed for rate status
			color := 0xff0000 // Red for increase (bad for borrowers)
			if rateChange < 0 {
				color = 0x00ff00 // Green for decrease (good for borrowers)
			}

			fields := []types.DiscordEmbedField{
				{
					Name:   fmt.Sprintf("**Current Rate:** %.2f%%", data.BorrowRate),
					Value:  " ",
					Inline: false,
				},
				{
					Name:   "Market Pair",
					Value:  vaultConfig.MarketPair,
					Inline: true,
				},
				{
					Name:   "Previous Rate",
					Value:  fmt.Sprintf("%.2f%%", lastRate),
					Inline: true,
				},
				{
					Name:   "Change",
					Value:  fmt.Sprintf("%+.2f%%", rateChange),
					Inline: true,
				},
			}

			embed := types.DiscordEmbed{
				Title:       fmt.Sprintf("Rate Status: %s", vaultConfig.Nickname),
				Description: "",
				Color:       color,
				Fields:      fields,
				Timestamp:   time.Now().Format(time.RFC3339),
				Footer: &types.DiscordEmbedFooter{
					Text: "SummerRateChecker",
				},
			}
			embeds = append(embeds, embed)
		}

		// Check if rate change exceeds threshold (both increases and decreases)
		if rateChangePercent >= vaultConfig.ThresholdPercent {
			// Create alert using the existing alert format
			alert := types.NewRateChangeAlert(
				vaultConfig.VaultID,
				vaultConfig.Nickname,
				vaultConfig.MarketPair,
				lastRate,
				data.BorrowRate,
			)

			// Send alert
			if err := m.sendDiscordAlert(alert, vaultConfig.ChannelID); err != nil {
				m.logger.Errorf("Failed to send Discord alert: %v", err)
			}
		}

		// Update last rate
		if err := m.storage.UpdateLastRate(vaultConfig.VaultID, data.BorrowRate); err != nil {
			m.logger.Errorf("Failed to update last rate for %s: %v", vaultConfig.VaultID, err)
		}
	}

	// Send status embeds to all unique channels
	channelMap := make(map[string]bool)
	for _, vault := range vaults {
		if !channelMap[vault.ChannelID] {
			payload := types.DiscordWebhookPayload{
				Embeds: embeds,
			}
			jsonData, err := json.Marshal(payload)
			if err != nil {
				m.logger.Errorf("Failed to marshal webhook payload: %v", err)
				continue
			}

			resp, err := m.httpClient.Post(m.config.Discord.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				m.logger.Errorf("Failed to send webhook: %v", err)
				continue
			}
			resp.Body.Close()

			channelMap[vault.ChannelID] = true
		}
	}

	return nil
}

func (m *Monitor) processMarketData(marketData *types.MarketData) error {
	vault, err := m.storage.GetVault(marketData.VaultID)
	if err != nil {
		return fmt.Errorf("failed to get vault config: %w", err)
	}

	if vault == nil {
		m.logger.Warnf("Received data for unknown vault: %s", marketData.VaultID)
		return nil
	}

	currentRate := marketData.BorrowRate
	previousRate, hasPreviousRate := m.storage.GetLastRate(marketData.VaultID)

	// Update the last rate
	if err := m.storage.UpdateLastRate(marketData.VaultID, currentRate); err != nil {
		m.logger.Errorf("Failed to update last rate for vault %s: %v", marketData.VaultID, err)
	}

	// Check if we should send an alert
	if hasPreviousRate {
		changePercent := math.Abs((currentRate - previousRate) / previousRate * 100)

		// Alert on both increases and decreases that exceed threshold
		if changePercent >= vault.ThresholdPercent {
			alert := types.NewRateChangeAlert(
				vault.VaultID,
				vault.Nickname,
				vault.MarketPair,
				previousRate,
				currentRate,
			)

			m.logger.Infof(
				"Rate change alert for %s: %.2f%% → %.2f%% (%+.2f%%)",
				vault.Nickname, previousRate, currentRate, alert.ChangePercent,
			)

			if err := m.sendDiscordAlert(alert, vault.ChannelID); err != nil {
				m.logger.Errorf("Failed to send Discord alert: %v", err)
			}
		}
	} else {
		m.logger.Infof("First check for vault %s (%s): %.2f%%", vault.VaultID, vault.Nickname, currentRate)
	}

	return nil
}

func (m *Monitor) sendDiscordAlert(alert *types.RateChangeAlert, channelID string) error {
	if m.config.Discord.WebhookURL == "" {
		m.logger.Warn("No webhook URL configured, skipping alert")
		return nil
	}

	payload := alert.ToDiscordEmbed()

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	resp, err := m.httpClient.Post(
		m.config.Discord.WebhookURL,
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (m *Monitor) sendRateAlert(vault *types.VaultConfig, oldRate, newRate float64) error {
	// Calculate rate change and direction
	rateChange := newRate - oldRate
	direction := "increased"
	if rateChange < 0 {
		direction = "decreased"
		rateChange = -rateChange
	}

	// Create alert message
	message := fmt.Sprintf("📈 Rate Alert: %s\n"+
		"Borrow rate %s from %.2f%% to %.2f%% (+%.2f%%)\n\n"+
		"Vault ID: %s\n"+
		"Previous Rate: %.2f%%\n"+
		"Current Rate: %.2f%%\n"+
		"Change: +%.2f%%",
		vault.Nickname,
		direction, oldRate, newRate, rateChange,
		vault.VaultID,
		oldRate, newRate, rateChange)

	// Send to Discord channel if configured
	if m.config.Discord.WebhookURL != "" {
		payload := map[string]interface{}{
			"content": message,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal webhook payload: %w", err)
		}

		resp, err := m.httpClient.Post(m.config.Discord.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to send webhook: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			return fmt.Errorf("webhook request failed with status %d", resp.StatusCode)
		}
	}

	return nil
}

func (m *Monitor) sendAlert(channelID, message string) {
	if m.config.Discord.WebhookURL == "" {
		m.logger.Warn("No webhook URL configured, skipping alert")
		return
	}

	payload := map[string]interface{}{
		"content": message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		m.logger.Errorf("Failed to marshal webhook payload: %v", err)
		return
	}

	resp, err := m.httpClient.Post(m.config.Discord.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		m.logger.Errorf("Failed to send webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.logger.Errorf("Webhook request failed with status %d", resp.StatusCode)
	}
}

func rateChangeVerb(rateChange float64) string {
	if rateChange > 0 {
		return "Increased"
	} else if rateChange < 0 {
		return "Decreased"
	} else {
		return "Stable"
	}
}
