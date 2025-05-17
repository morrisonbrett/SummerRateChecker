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
			// Also set this as the last alert rate
			vaultConfig.LastAlertRate = data.BorrowRate
			if err := m.storage.AddVault(vaultConfig); err != nil {
				m.logger.Errorf("Failed to update last alert rate for %s: %v", vaultConfig.VaultID, err)
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

		// Calculate rate change in percentage points from the last alert rate
		// If LastAlertRate is not set (0), use the last check rate
		compareRate := vaultConfig.LastAlertRate
		if compareRate == 0 {
			compareRate = lastRate
		}
		rateChange := data.BorrowRate - compareRate
		rateChangePoints := math.Abs(rateChange) // This is now in percentage points

		// Only send messages if there's an actual change that exceeds the threshold
		if rateChangePoints >= vaultConfig.ThresholdPercent {
			// Create alert using the existing alert format
			alert := types.NewRateChangeAlert(
				vaultConfig.VaultID,
				vaultConfig.Nickname,
				vaultConfig.MarketPair,
				compareRate, // Use the comparison rate (last alert or last check)
				data.BorrowRate,
			)

			// Send alert
			if err := m.sendDiscordAlert(alert, vaultConfig.ChannelID); err != nil {
				m.logger.Errorf("Failed to send Discord alert: %v", err)
			}

			// Update the last alert rate
			vaultConfig.LastAlertRate = data.BorrowRate
			if err := m.storage.AddVault(vaultConfig); err != nil {
				m.logger.Errorf("Failed to update last alert rate for %s: %v", vaultConfig.VaultID, err)
			}
		}

		// Update last rate regardless of whether we sent an alert
		if err := m.storage.UpdateLastRate(vaultConfig.VaultID, data.BorrowRate); err != nil {
			m.logger.Errorf("Failed to update last rate for %s: %v", vaultConfig.VaultID, err)
		}
	}

	// Only send status embeds if we have any to send
	if len(embeds) > 0 {
		// Send status embeds to all unique channels
		channelMap := make(map[string]bool)
		for _, vault := range vaults {
			if !channelMap[vault.ChannelID] && vault.WebhookURL != "" {
				payload := types.DiscordWebhookPayload{
					Embeds: embeds,
				}
				jsonData, err := json.Marshal(payload)
				if err != nil {
					m.logger.Errorf("Failed to marshal webhook payload: %v", err)
					continue
				}

				resp, err := m.httpClient.Post(vault.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
				if err != nil {
					m.logger.Errorf("Failed to send webhook: %v", err)
					continue
				}
				resp.Body.Close()

				channelMap[vault.ChannelID] = true
			}
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
		changePoints := math.Abs(currentRate - previousRate) // This is now in percentage points

		// Alert on both increases and decreases that exceed threshold
		if changePoints >= vault.ThresholdPercent {
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
	vault, err := m.storage.GetVault(alert.VaultID)
	if err != nil {
		return fmt.Errorf("failed to get vault config: %w", err)
	}

	if vault == nil {
		return fmt.Errorf("vault %s not found", alert.VaultID)
	}

	if vault.WebhookURL == "" {
		m.logger.Warnf("No webhook URL configured for vault %s, skipping alert", alert.VaultID)
		return nil
	}

	payload := alert.ToDiscordEmbed()

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	resp, err := m.httpClient.Post(
		vault.WebhookURL,
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

func (m *Monitor) sendAlert(channelID, message string) {
	vaults, err := m.storage.GetAllVaults()
	if err != nil {
		m.logger.Errorf("Failed to get vaults: %v", err)
		return
	}

	// Find vaults that use this channel
	for _, vault := range vaults {
		if vault.ChannelID == channelID && vault.WebhookURL != "" {
			payload := map[string]interface{}{
				"content": message,
			}

			jsonData, err := json.Marshal(payload)
			if err != nil {
				m.logger.Errorf("Failed to marshal webhook payload: %v", err)
				continue
			}

			resp, err := m.httpClient.Post(vault.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				m.logger.Errorf("Failed to send webhook: %v", err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				m.logger.Errorf("Webhook request failed with status %d", resp.StatusCode)
			}
		}
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
