package types

import (
	"fmt"
	"math"
	"time"
)

// VaultConfig represents a vault being monitored
type VaultConfig struct {
	VaultID          string    `json:"vault_id"`
	Nickname         string    `json:"nickname"`
	ThresholdPercent float64   `json:"threshold_percent"`
	ChannelID        string    `json:"channel_id"`
	WebhookURL       string    `json:"webhook_url,omitempty"` // Discord webhook URL for this vault's channel
	CreatedAt        time.Time `json:"created_at"`
	MorphoMarketKey  string    `json:"morpho_market_key,omitempty"` // The Morpho market unique key for this vault
	MarketPair       string    `json:"market_pair,omitempty"`       // The market pair (e.g., "WBTC-USDC")
	LastAlertRate    float64   `json:"last_alert_rate,omitempty"`   // The rate that last triggered an alert
}

// MarketData represents the current market data for a vault
type MarketData struct {
	VaultID         string    `json:"vault_id"`
	MorphoMarketKey string    `json:"morpho_market_key"`
	BorrowRate      float64   `json:"borrow_rate"`
	SupplyRate      float64   `json:"supply_rate"`
	Timestamp       time.Time `json:"timestamp"`
}

type RateChangeAlert struct {
	VaultID       string    `json:"vault_id"`
	Nickname      string    `json:"nickname"`
	MarketPair    string    `json:"market_pair,omitempty"` // The market pair (e.g., "WBTC-USDC")
	PreviousRate  float64   `json:"previous_rate"`
	CurrentRate   float64   `json:"current_rate"`
	ChangePercent float64   `json:"change_percent"`
	Timestamp     time.Time `json:"timestamp"`
}

func NewRateChangeAlert(vaultID, nickname, marketPair string, prevRate, currRate float64) *RateChangeAlert {
	changePoints := currRate - prevRate // This is now in percentage points
	return &RateChangeAlert{
		VaultID:       vaultID,
		Nickname:      nickname,
		MarketPair:    marketPair,
		PreviousRate:  prevRate,
		CurrentRate:   currRate,
		ChangePercent: changePoints, // This is now in percentage points
		Timestamp:     time.Now(),
	}
}

func (r *RateChangeAlert) ToDiscordMessage() string {
	icon := "📈"
	direction := "increased"
	if r.ChangePercent < 0 {
		icon = "📉"
		direction = "decreased"
	}

	return fmt.Sprintf(
		"%s **Rate Alert: %s**\n\n"+
			"**Current Rate: %.2f%%**\n"+
			"Previous Rate: %.2f%%\n"+
			"Change: %s by %.2f percentage points\n\n"+
			"<t:%d:R>",
		icon,
		r.Nickname,
		r.CurrentRate,
		r.PreviousRate,
		direction,
		math.Abs(r.ChangePercent),
		r.Timestamp.Unix(),
	)
}

type DiscordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Color       int                 `json:"color"`
	Fields      []DiscordEmbedField `json:"fields"`
	Timestamp   string              `json:"timestamp"`
	Footer      *DiscordEmbedFooter `json:"footer,omitempty"`
}

type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type DiscordEmbedFooter struct {
	Text string `json:"text"`
}

type DiscordWebhookPayload struct {
	Embeds []DiscordEmbed `json:"embeds"`
}

func (r *RateChangeAlert) ToDiscordEmbed() *DiscordWebhookPayload {
	color := 0xff0000 // Red for increase (bad for borrowers)
	if r.ChangePercent < 0 {
		color = 0x00ff00 // Green for decrease (good for borrowers)
	}

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("Rate Alert: %s", r.Nickname),
		Description: r.ToDiscordMessage(),
		Color:       color,
		Fields: []DiscordEmbedField{
			{
				Name:   "Vault ID",
				Value:  r.VaultID,
				Inline: true,
			},
			{
				Name:   "Market Pair",
				Value:  r.MarketPair,
				Inline: true,
			},
		},
		Timestamp: r.Timestamp.Format(time.RFC3339),
		Footer: &DiscordEmbedFooter{
			Text: "SummerRateChecker",
		},
	}

	return &DiscordWebhookPayload{
		Embeds: []DiscordEmbed{embed},
	}
}
