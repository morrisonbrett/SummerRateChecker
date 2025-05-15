package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/morrisonbrett/SummerRateChecker/internal/types"
)

type FileStorage struct {
	mu         sync.RWMutex
	vaults     map[string]*types.VaultConfig
	lastRates  map[string]float64
	dataDir    string
	vaultsFile string
	ratesFile  string
}

func NewFileStorage(dataDir string) (*FileStorage, error) {
	if dataDir == "" {
		dataDir = "data"
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	fs := &FileStorage{
		vaults:     make(map[string]*types.VaultConfig),
		lastRates:  make(map[string]float64),
		dataDir:    dataDir,
		vaultsFile: filepath.Join(dataDir, "vaults.json"),
		ratesFile:  filepath.Join(dataDir, "rates.json"),
	}

	// Load existing data
	if err := fs.loadFromDisk(); err != nil {
		return nil, fmt.Errorf("failed to load data from disk: %w", err)
	}

	return fs, nil
}

func (fs *FileStorage) AddVault(vault *types.VaultConfig) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	vault.CreatedAt = time.Now()
	fs.vaults[vault.VaultID] = vault
	return fs.saveVaultsToDisk()
}

func (fs *FileStorage) RemoveVault(vaultID string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	delete(fs.vaults, vaultID)
	delete(fs.lastRates, vaultID)

	if err := fs.saveVaultsToDisk(); err != nil {
		return err
	}
	return fs.saveRatesToDisk()
}

func (fs *FileStorage) GetVault(vaultID string) (*types.VaultConfig, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	vault, exists := fs.vaults[vaultID]
	if !exists {
		return nil, nil
	}
	return vault, nil
}

func (fs *FileStorage) GetAllVaults() ([]*types.VaultConfig, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	vaults := make([]*types.VaultConfig, 0, len(fs.vaults))
	for _, vault := range fs.vaults {
		vaults = append(vaults, vault)
	}
	return vaults, nil
}

func (fs *FileStorage) UpdateLastRate(vaultID string, rate float64) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.lastRates[vaultID] = rate
	return fs.saveRatesToDisk()
}

func (fs *FileStorage) GetLastRate(vaultID string) (float64, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	rate, exists := fs.lastRates[vaultID]
	return rate, exists
}

func (fs *FileStorage) GetAllLastRates() map[string]float64 {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	rates := make(map[string]float64)
	for k, v := range fs.lastRates {
		rates[k] = v
	}
	return rates
}

func (fs *FileStorage) loadFromDisk() error {
	// Load vaults
	if err := fs.loadVaultsFromDisk(); err != nil {
		return err
	}

	// Load rates
	if err := fs.loadRatesFromDisk(); err != nil {
		return err
	}

	return nil
}

func (fs *FileStorage) loadVaultsFromDisk() error {
	if _, err := os.Stat(fs.vaultsFile); os.IsNotExist(err) {
		// File doesn't exist, start with empty vaults
		return nil
	}

	data, err := os.ReadFile(fs.vaultsFile)
	if err != nil {
		return fmt.Errorf("failed to read vaults file: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, &fs.vaults); err != nil {
		return fmt.Errorf("failed to unmarshal vaults: %w", err)
	}

	return nil
}

func (fs *FileStorage) loadRatesFromDisk() error {
	if _, err := os.Stat(fs.ratesFile); os.IsNotExist(err) {
		// File doesn't exist, start with empty rates
		return nil
	}

	data, err := os.ReadFile(fs.ratesFile)
	if err != nil {
		return fmt.Errorf("failed to read rates file: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, &fs.lastRates); err != nil {
		return fmt.Errorf("failed to unmarshal rates: %w", err)
	}

	return nil
}

func (fs *FileStorage) saveVaultsToDisk() error {
	data, err := json.MarshalIndent(fs.vaults, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal vaults: %w", err)
	}

	if err := os.WriteFile(fs.vaultsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write vaults file: %w", err)
	}

	return nil
}

func (fs *FileStorage) saveRatesToDisk() error {
	data, err := json.MarshalIndent(fs.lastRates, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal rates: %w", err)
	}

	if err := os.WriteFile(fs.ratesFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write rates file: %w", err)
	}

	return nil
}
