package storage

import (
	"sync"
	"time"

	"github.com/morrisonbrett/SummerRateChecker/internal/types"
)

type Storage interface {
	AddVault(vault *types.VaultConfig) error
	RemoveVault(vaultID string) error
	GetVault(vaultID string) (*types.VaultConfig, error)
	GetAllVaults() ([]*types.VaultConfig, error)
	UpdateLastRate(vaultID string, rate float64) error
	GetLastRate(vaultID string) (float64, bool)
	GetAllLastRates() map[string]float64
}

type InMemoryStorage struct {
	mu        sync.RWMutex
	vaults    map[string]*types.VaultConfig
	lastRates map[string]float64
}

func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		vaults:    make(map[string]*types.VaultConfig),
		lastRates: make(map[string]float64),
	}
}

func (s *InMemoryStorage) AddVault(vault *types.VaultConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	vault.CreatedAt = time.Now()
	s.vaults[vault.VaultID] = vault
	return nil
}

func (s *InMemoryStorage) RemoveVault(vaultID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.vaults, vaultID)
	delete(s.lastRates, vaultID)
	return nil
}

func (s *InMemoryStorage) GetVault(vaultID string) (*types.VaultConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	vault, exists := s.vaults[vaultID]
	if !exists {
		return nil, nil
	}
	return vault, nil
}

func (s *InMemoryStorage) GetAllVaults() ([]*types.VaultConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	vaults := make([]*types.VaultConfig, 0, len(s.vaults))
	for _, vault := range s.vaults {
		vaults = append(vaults, vault)
	}
	return vaults, nil
}

func (s *InMemoryStorage) UpdateLastRate(vaultID string, rate float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastRates[vaultID] = rate
	return nil
}

func (s *InMemoryStorage) GetLastRate(vaultID string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rate, exists := s.lastRates[vaultID]
	return rate, exists
}

func (s *InMemoryStorage) GetAllLastRates() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rates := make(map[string]float64)
	for k, v := range s.lastRates {
		rates[k] = v
	}
	return rates
}
