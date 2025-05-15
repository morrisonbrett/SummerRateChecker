package morpho

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/machinebox/graphql"
	"github.com/morrisonbrett/SummerRateChecker/internal/types"
	"go.uber.org/zap"
)

type Client struct {
	client *graphql.Client
	logger *zap.SugaredLogger
}

// Market data from the API
type MarketResponse struct {
	MarketByUniqueKey struct {
		UniqueKey string `json:"uniqueKey"`
		State     struct {
			BorrowApy float64 `json:"borrowApy"`
			SupplyApy float64 `json:"supplyApy"`
		} `json:"state"`
		LoanAsset struct {
			Symbol string `json:"symbol"`
		} `json:"loanAsset"`
		CollateralAsset struct {
			Symbol string `json:"symbol"`
		} `json:"collateralAsset"`
	} `json:"marketByUniqueKey"`
}

// Market list response for vault ID lookup
type MarketsResponse struct {
	Markets struct {
		Items []struct {
			ID        string `json:"id"`
			UniqueKey string `json:"uniqueKey"`
			LoanAsset struct {
				Symbol   string `json:"symbol"`
				Address  string `json:"address"`
				Decimals int    `json:"decimals"`
			} `json:"loanAsset"`
			CollateralAsset struct {
				Symbol   string `json:"symbol"`
				Address  string `json:"address"`
				Decimals int    `json:"decimals"`
			} `json:"collateralAsset"`
			State struct {
				BorrowApy float64 `json:"borrowApy"`
				SupplyApy float64 `json:"supplyApy"`
			} `json:"state"`
		} `json:"items"`
	} `json:"markets"`
}

func NewClient(apiURL string, logger *zap.SugaredLogger) *Client {
	return &Client{
		client: graphql.NewClient(apiURL),
		logger: logger,
	}
}

func (c *Client) GetMarketData(ctx context.Context, vaultID string) (*types.MarketData, error) {
	c.logger.Infof("Fetching market data for vault ID: %s", vaultID)

	// Try vault ID directly as unique key first
	marketData, err := c.fetchMarketByUniqueKey(ctx, vaultID, vaultID)
	if err == nil {
		return marketData, nil
	}

	c.logger.Warnf("Vault ID %s not found as unique key, searching in markets list...", vaultID)

	// If that fails, search for the vault ID in the markets list
	uniqueKey, err := c.findUniqueKeyBySearch(ctx, vaultID)
	if err != nil {
		return nil, fmt.Errorf("failed to find unique key for vault %s: %w", vaultID, err)
	}

	c.logger.Infof("Found unique key %s for vault %s", uniqueKey, vaultID)

	// Now fetch with the discovered unique key
	return c.fetchMarketByUniqueKey(ctx, uniqueKey, vaultID)
}

func (c *Client) fetchMarketByUniqueKey(ctx context.Context, uniqueKey string, originalVaultID string) (*types.MarketData, error) {
	req := graphql.NewRequest(`
		query GetMarketData($uniqueKey: String!) {
			marketByUniqueKey(uniqueKey: $uniqueKey, chainId: 1) {
				uniqueKey
				loanAsset {
					symbol
				}
				collateralAsset {
					symbol
				}
				state {
					borrowApy
					supplyApy
				}
			}
		}
	`)

	req.Var("uniqueKey", uniqueKey)

	var resp MarketResponse
	if err := c.client.Run(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("GraphQL API error for unique key %s: %w", uniqueKey, err)
	}

	// Check if we got valid data
	if resp.MarketByUniqueKey.UniqueKey == "" {
		return nil, fmt.Errorf("no market data found for unique key %s", uniqueKey)
	}

	// Convert from decimal to percentage
	borrowRate := resp.MarketByUniqueKey.State.BorrowApy * 100
	supplyRate := resp.MarketByUniqueKey.State.SupplyApy * 100

	c.logger.Infof("✅ Successfully fetched data for unique key %s (%s/%s): Borrow=%.4f%%, Supply=%.4f%%",
		uniqueKey,
		resp.MarketByUniqueKey.CollateralAsset.Symbol,
		resp.MarketByUniqueKey.LoanAsset.Symbol,
		borrowRate,
		supplyRate)

	return &types.MarketData{
		VaultID:         originalVaultID, // Keep the original vault ID
		MorphoMarketKey: uniqueKey,       // Store the actual unique key
		BorrowRate:      borrowRate,
		SupplyRate:      supplyRate,
		Timestamp:       time.Now(),
	}, nil
}

// findUniqueKeyBySearch searches through all markets to find a matching vault ID
func (c *Client) findUniqueKeyBySearch(ctx context.Context, vaultID string) (string, error) {
	c.logger.Infof("Searching for vault ID %s in markets list", vaultID)

	// Get all markets and search for our vault ID
	req := graphql.NewRequest(`
		query GetAllMarkets {
			markets(first: 1000, where: { chainId_in: [1] }) {
				items {
					uniqueKey
					loanAsset {
						symbol
					}
					collateralAsset {
						symbol
					}
					state {
						borrowApy
						supplyApy
					}
				}
			}
		}
	`)

	var resp MarketsResponse
	if err := c.client.Run(ctx, req, &resp); err != nil {
		return "", fmt.Errorf("failed to fetch markets list: %w", err)
	}

	c.logger.Infof("Searching through %d markets for vault ID %s", len(resp.Markets.Items), vaultID)

	// Search strategies:
	// 1. Unique key contains the vault ID
	// 2. Unique key ends with vault ID
	// 3. Other patterns...

	for _, market := range resp.Markets.Items {
		// Check if unique key contains the vault ID
		if strings.Contains(market.UniqueKey, vaultID) {
			c.logger.Infof("Found match: %s contains %s (%s/%s)",
				market.UniqueKey, vaultID,
				market.CollateralAsset.Symbol, market.LoanAsset.Symbol)
			return market.UniqueKey, nil
		}

		// Check if unique key ends with vault ID (common pattern)
		if strings.HasSuffix(market.UniqueKey, vaultID) {
			c.logger.Infof("Found match: %s ends with %s (%s/%s)",
				market.UniqueKey, vaultID,
				market.CollateralAsset.Symbol, market.LoanAsset.Symbol)
			return market.UniqueKey, nil
		}
	}

	// If no match found, log some markets for debugging
	c.logger.Errorf("No unique key found for vault ID %s", vaultID)
	c.logger.Info("Available markets (first 10):")
	for i, market := range resp.Markets.Items {
		if i >= 10 {
			break
		}
		c.logger.Infof("  %s (%s/%s)", market.UniqueKey,
			market.CollateralAsset.Symbol, market.LoanAsset.Symbol)
	}

	return "", fmt.Errorf("vault ID %s not found in any unique keys", vaultID)
}

func (c *Client) GetMultipleMarkets(ctx context.Context, vaults []*types.VaultConfig) ([]*types.MarketData, error) {
	results := make([]*types.MarketData, 0, len(vaults))
	var errors []string

	for _, vault := range vaults {
		data, err := c.GetMarketDataByVaultID(ctx, vault.VaultID, vault.MorphoMarketKey, vault.MarketPair)
		if err != nil {
			c.logger.Errorf("Failed to get data for vault %s: %v", vault.VaultID, err)
			errors = append(errors, fmt.Sprintf("vault %s: %v", vault.VaultID, err))
			continue
		}

		// If we found a market key and it's not stored, update it
		if vault.MorphoMarketKey == "" && data.MorphoMarketKey != "" {
			vault.MorphoMarketKey = data.MorphoMarketKey
			c.logger.Infof("Discovered and stored Morpho market key %s for vault %s",
				vault.MorphoMarketKey, vault.VaultID)
		}

		results = append(results, data)
	}

	// If we have both results and errors, log the errors but return the successful results
	if len(errors) > 0 {
		c.logger.Warnf("Some vaults failed: %v", strings.Join(errors, "; "))
	}

	// If all vaults failed, return an error
	if len(results) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("all vault requests failed: %s", strings.Join(errors, "; "))
	}

	return results, nil
}

func (c *Client) GetMarketDataByVaultID(ctx context.Context, vaultID string, morphoMarketKey string, marketPair string) (*types.MarketData, error) {
	c.logger.Infof("Fetching market data for vault ID: %s (market pair: %s)", vaultID, marketPair)

	// If we have a stored Morpho market key, use it directly
	if morphoMarketKey != "" {
		c.logger.Infof("Using stored Morpho market key: %s", morphoMarketKey)
		return c.fetchMarketByUniqueKey(ctx, morphoMarketKey, vaultID)
	}

	// Otherwise try to find the unique key
	uniqueKey, err := c.findUniqueKeyByVaultID(ctx, vaultID, marketPair)
	if err != nil {
		return nil, fmt.Errorf("failed to find unique key for vault %s: %w", vaultID, err)
	}

	// Now fetch with the discovered unique key
	return c.fetchMarketByUniqueKey(ctx, uniqueKey, vaultID)
}

// findUniqueKeyByVaultID searches for the unique key that corresponds to a vault ID
func (c *Client) findUniqueKeyByVaultID(ctx context.Context, vaultID string, marketPair string) (string, error) {
	c.logger.Infof("Searching for unique key for vault ID %s (market pair: %s)", vaultID, marketPair)

	// Get all markets with more detailed information
	req := graphql.NewRequest(`
		query GetAllMarkets {
			markets(first: 1000, where: { chainId_in: [1] }) {
				items {
					uniqueKey
					id
					loanAsset {
						symbol
						address
						decimals
					}
					collateralAsset {
						symbol
						address
						decimals
					}
					state {
						borrowApy
						supplyApy
					}
				}
			}
		}
	`)

	var resp MarketsResponse
	if err := c.client.Run(ctx, req, &resp); err != nil {
		return "", fmt.Errorf("failed to fetch markets list: %w", err)
	}

	c.logger.Infof("Searching through %d markets for vault ID %s", len(resp.Markets.Items), vaultID)

	// Log all markets for debugging
	c.logger.Debug("Available markets:")
	for _, market := range resp.Markets.Items {
		c.logger.Debugf("Market: ID=%s, UniqueKey=%s, Pair=%s/%s, LoanAddr=%s, CollAddr=%s",
			market.ID,
			market.UniqueKey,
			market.CollateralAsset.Symbol,
			market.LoanAsset.Symbol,
			market.LoanAsset.Address,
			market.CollateralAsset.Address)
	}

	// If we have a market pair, try to find an exact match first
	if marketPair != "" {
		// Split the market pair into collateral and loan assets
		parts := strings.Split(marketPair, "-")
		if len(parts) == 2 {
			collateralSymbol := parts[0]
			loanSymbol := parts[1]

			// Look for an exact match of the market pair
			for _, market := range resp.Markets.Items {
				if market.CollateralAsset.Symbol == collateralSymbol && market.LoanAsset.Symbol == loanSymbol {
					c.logger.Infof("Found exact market pair match: %s (%s/%s)",
						market.UniqueKey,
						market.CollateralAsset.Symbol,
						market.LoanAsset.Symbol)
					return market.UniqueKey, nil
				}
			}
		}
	}

	// Try different matching strategies
	for _, market := range resp.Markets.Items {
		// Strategy 1: Check if market ID matches vault ID
		if market.ID == vaultID {
			c.logger.Infof("Found match by market ID: %s (%s/%s)",
				market.UniqueKey,
				market.CollateralAsset.Symbol, market.LoanAsset.Symbol)
			return market.UniqueKey, nil
		}

		// Strategy 2: Check if unique key contains the vault ID
		if strings.Contains(market.UniqueKey, vaultID) {
			c.logger.Infof("Found match by unique key contains: %s (%s/%s)",
				market.UniqueKey,
				market.CollateralAsset.Symbol, market.LoanAsset.Symbol)
			return market.UniqueKey, nil
		}

		// Strategy 3: Check if unique key ends with vault ID
		if strings.HasSuffix(market.UniqueKey, vaultID) {
			c.logger.Infof("Found match by unique key suffix: %s (%s/%s)",
				market.UniqueKey,
				market.CollateralAsset.Symbol, market.LoanAsset.Symbol)
			return market.UniqueKey, nil
		}

		// Strategy 4: Check if vault ID is part of the asset addresses
		if strings.Contains(market.LoanAsset.Address, vaultID) ||
			strings.Contains(market.CollateralAsset.Address, vaultID) {
			c.logger.Infof("Found match by asset address: %s (%s/%s)",
				market.UniqueKey,
				market.CollateralAsset.Symbol, market.LoanAsset.Symbol)
			return market.UniqueKey, nil
		}

		// Strategy 5: Check if vault ID is a substring of the market ID
		if strings.Contains(market.ID, vaultID) {
			c.logger.Infof("Found match by market ID contains: %s (%s/%s)",
				market.UniqueKey,
				market.CollateralAsset.Symbol, market.LoanAsset.Symbol)
			return market.UniqueKey, nil
		}
	}

	// If no match found, log detailed information about available markets
	c.logger.Errorf("No unique key found for vault ID %s", vaultID)
	c.logger.Info("Available markets (first 10):")
	for i, market := range resp.Markets.Items {
		if i >= 10 {
			break
		}
		c.logger.Infof("  Market ID: %s, Unique Key: %s, Pair: %s/%s",
			market.ID,
			market.UniqueKey,
			market.CollateralAsset.Symbol, market.LoanAsset.Symbol)
	}

	return "", fmt.Errorf("vault ID %s not found in any markets", vaultID)
}
