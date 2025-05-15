package morpho

import (
	"fmt"
	"net/url"
	"strings"
)

// VaultURLInfo contains information extracted from a Summer.fi vault URL
type VaultURLInfo struct {
	VaultID    string // The vault ID (e.g., "1234")
	MarketPair string // The market pair (e.g., "WBTC-USDC")
}

// ParseVaultURL extracts vault information from a Summer.fi URL
// Example URL: https://pro.summer.fi/ethereum/morphoblue/borrow/WBTC-USDC/1234#overview
func ParseVaultURL(urlStr string) (*VaultURLInfo, error) {
	// Parse the URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Check if it's a Summer.fi URL
	if !strings.Contains(parsedURL.Host, "summer.fi") {
		return nil, fmt.Errorf("not a Summer.fi URL")
	}

	// Split the path into components
	// Expected format: /ethereum/morphoblue/borrow/WBTC-USDC/1234
	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) < 5 {
		return nil, fmt.Errorf("invalid URL format: expected at least 5 path components")
	}

	// Extract the market pair and vault ID
	// The last two components should be the market pair and vault ID
	marketPair := pathParts[len(pathParts)-2]
	vaultID := pathParts[len(pathParts)-1]

	// Validate the components
	if marketPair == "" || vaultID == "" {
		return nil, fmt.Errorf("invalid URL format: missing market pair or vault ID")
	}

	// Validate market pair format (should contain a hyphen)
	if !strings.Contains(marketPair, "-") {
		return nil, fmt.Errorf("invalid market pair format: should be like 'WBTC-USDC'")
	}

	// Validate vault ID (should be numeric)
	if !isNumeric(vaultID) {
		return nil, fmt.Errorf("invalid vault ID: should be numeric")
	}

	return &VaultURLInfo{
		VaultID:    vaultID,
		MarketPair: marketPair,
	}, nil
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
