// file: /core/ultrastable_integration.go
// description: Integration of proprietary UltraStable token modules
// module: Blockchain Core
// License: MIT
// Author: Andrew Donelson
// Copyright 2025 Andrew Donelson

package core

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"

	proprietary "github.com/AndrewDonelson/o2ul-proprietary"
	"github.com/AndrewDonelson/o2ul-proprietary/seigniorage"
	"github.com/AndrewDonelson/o2ul-proprietary/ultrastable"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

// UltraStableManager handles all UltraStable token operations
type UltraStableManager struct {
	blockchain *BlockChain
	config     *params.ChainConfig

	// Proprietary module manager
	proprietary *proprietary.Manager

	// Update management
	updateLock     sync.RWMutex
	lastUpdateTime time.Time

	// Event subscription
	scope      event.SubscriptionScope
	updateFeed event.Feed
	adjustFeed event.Feed

	quit chan struct{}
}

// NewUltraStableManager creates a new manager instance
func NewUltraStableManager(blockchain *BlockChain, config *params.ChainConfig) *UltraStableManager {
	manager := &UltraStableManager{
		blockchain:  blockchain,
		config:      config,
		proprietary: proprietary.NewManager(),
		quit:        make(chan struct{}),
	}

	return manager
}

// Start initializes the UltraStable token system
func (m *UltraStableManager) Start() error {
	// Start proprietary modules
	if err := m.proprietary.Start(); err != nil {
		return err
	}

	// Start update worker
	go m.updateWorker()

	log.Info("UltraStable token system started")
	return nil
}

// Stop halts the UltraStable token system
func (m *UltraStableManager) Stop() {
	close(m.quit)
	m.proprietary.Stop()
	log.Info("UltraStable token system stopped")
}

// updateWorker handles periodic updates to the UltraStable token
func (m *UltraStableManager) updateWorker() {
	// Check for updates every minute
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.quit:
			return
		case <-ticker.C:
			m.checkForUpdates()
		}
	}
}

// checkForUpdates determines if an update is needed
func (m *UltraStableManager) checkForUpdates() {
	m.updateLock.RLock()
	lastUpdate := m.lastUpdateTime
	m.updateLock.RUnlock()

	// Get proprietary update time
	proprietaryUpdate := m.proprietary.GetLastUpdateTime()

	// If proprietary modules have newer data, trigger update
	if proprietaryUpdate.After(lastUpdate) {
		log.Info("New UltraStable data available, triggering update",
			"lastUpdate", lastUpdate,
			"newUpdate", proprietaryUpdate)

		m.ProcessUpdate()
	}
}

// ProcessUpdate applies the latest UltraStable token updates
func (m *UltraStableManager) ProcessUpdate() {
	// Get current state
	statedb, err := m.blockchain.State()
	if err != nil {
		log.Error("Failed to get blockchain state", "error", err)
		return
	}

	// Get current supply
	supplyBytes := statedb.GetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_current_supply"))
	currentSupply := new(big.Int).SetBytes(supplyBytes[:])

	// Get Value token price
	priceBytes := statedb.GetState(
		params.O2ULTokenSystemAddress,
		common.HexToHash("value_token_price"))
	valueTokenPrice := new(big.Int).SetBytes(priceBytes[:])
	if valueTokenPrice.Cmp(big.NewInt(0)) == 0 {
		valueTokenPrice = big.NewInt(1e18) // Default 1.0 if not set
	}

	// Get market volatility (0-100)
	volatilityBytes := statedb.GetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("market_volatility"))
	volatility := uint8(new(big.Int).SetBytes(volatilityBytes[:]).Uint64())

	// Calculate supply adjustment
	adjustment := m.proprietary.CalculateSupplyAdjustment(
		currentSupply, valueTokenPrice, volatility)

	// Emit event
	m.updateFeed.Send(adjustment)

	// Store current values in state
	targetValue := m.proprietary.GetTargetStableValue()
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_target_value"),
		common.BytesToHash(targetValue.Bytes()))

	currentValue := m.proprietary.GetCurrentStableValue()
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_current_value"),
		common.BytesToHash(currentValue.Bytes()))

	// Store last update time
	updateTime := time.Now().Unix()
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_last_update_time"),
		common.BytesToHash(big.NewInt(updateTime).Bytes()))

	// Update local timestamp
	m.updateLock.Lock()
	m.lastUpdateTime = time.Now()
	m.updateLock.Unlock()

	log.Info("Processed UltraStable update",
		"targetValue", targetValue,
		"currentValue", currentValue,
		"adjustmentType", adjustment.Type,
		"adjustmentAmount", adjustment.Amount)
}

// ApplySupplyAdjustment executes a seigniorage operation
func (m *UltraStableManager) ApplySupplyAdjustment(
	adjustment seigniorage.AdjustmentResult,
	treasuryAddr common.Address) error {

	// If no adjustment needed, return early
	if adjustment.Type == seigniorage.None {
		return nil
	}

	// Get current state
	statedb, err := m.blockchain.State()
	if err != nil {
		return err
	}

	// Check minimum supply
	minSupplyBytes := statedb.GetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_minimum_supply"))
	minSupply := new(big.Int).SetBytes(minSupplyBytes[:])

	// Get Value token balance of treasury
	treasuryBalance := statedb.GetBalance(treasuryAddr)

	// Check if adjustment is possible
	possible, reason := m.proprietary.IsAdjustmentPossible(
		adjustment,
		treasuryBalance.ToBig(),
		minSupply)

	if !possible {
		log.Warn("Supply adjustment not possible", "reason", reason)
		return nil
	}

	// Apply adjustment based on type
	if adjustment.Type == seigniorage.Expansion {
		// For expansion, Value tokens are burned from treasury
		// and new UltraStable tokens are minted

		// Convert big.Int to uint256.Int for state operations
		valueAmount, overflow := uint256.FromBig(adjustment.ValueTokens)
		if overflow {
			return errors.New("value token amount overflow")
		}

		// Define a reason constant directly here as a workaround
		const stablecoinAdjustmentReason = 1 // This matches the iota value from proprietary package

		// Burn Value tokens from treasury
		statedb.SubBalance(treasuryAddr, valueAmount, stablecoinAdjustmentReason)

		// Update total supply
		supplyBytes := statedb.GetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash("ultrastable_current_supply"))
		currentSupply := new(big.Int).SetBytes(supplyBytes[:])

		newSupply := new(big.Int).Add(currentSupply, adjustment.Amount)
		statedb.SetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash("ultrastable_current_supply"),
			common.BytesToHash(newSupply.Bytes()))

		log.Info("Applied expansion adjustment",
			"amount", adjustment.Amount,
			"valueTokensBurned", adjustment.ValueTokens,
			"newSupply", newSupply)
	} else if adjustment.Type == seigniorage.Contraction {
		// For contraction, UltraStable tokens are burned
		// and Value tokens are minted to treasury

		// Convert big.Int to uint256.Int for state operations
		valueAmount, overflow := uint256.FromBig(adjustment.ValueTokens)
		if overflow {
			return errors.New("value token amount overflow")
		}

		// Define a reason constant directly here as a workaround
		const stablecoinAdjustmentReason = 1 // This matches the iota value from proprietary package

		// Add Value tokens to treasury
		statedb.AddBalance(treasuryAddr, valueAmount, stablecoinAdjustmentReason)

		// Update total supply
		supplyBytes := statedb.GetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash("ultrastable_current_supply"))
		currentSupply := new(big.Int).SetBytes(supplyBytes[:])

		newSupply := new(big.Int).Sub(currentSupply, adjustment.Amount)
		statedb.SetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash("ultrastable_current_supply"),
			common.BytesToHash(newSupply.Bytes()))

		log.Info("Applied contraction adjustment",
			"amount", adjustment.Amount,
			"valueTokensMinted", adjustment.ValueTokens,
			"newSupply", newSupply)
	}

	// Update adjustment history
	m.updateAdjustmentHistory(adjustment)

	// Emit adjustment event
	m.adjustFeed.Send(adjustment)

	return nil
}

// updateAdjustmentHistory adds the adjustment to historical records
func (m *UltraStableManager) updateAdjustmentHistory(adjustment seigniorage.AdjustmentResult) {
	statedb, err := m.blockchain.State()
	if err != nil {
		log.Error("Failed to get state for history update", "error", err)
		return
	}

	// Get current adjustment count
	countBytes := statedb.GetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("adjustment_history_count"))
	count := new(big.Int).SetBytes(countBytes[:])

	// Increment count
	newCount := new(big.Int).Add(count, big.NewInt(1))
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("adjustment_history_count"),
		common.BytesToHash(newCount.Bytes()))

	// Store adjustment details
	prefix := "adjustment_" + count.String() + "_"

	// Type
	var typeValue *big.Int
	switch adjustment.Type {
	case seigniorage.Expansion:
		typeValue = big.NewInt(1)
	case seigniorage.Contraction:
		typeValue = big.NewInt(2)
	default:
		typeValue = big.NewInt(0)
	}

	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash(prefix+"type"),
		common.BytesToHash(typeValue.Bytes()))

	// Amount
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash(prefix+"amount"),
		common.BytesToHash(adjustment.Amount.Bytes()))

	// Value tokens
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash(prefix+"value_tokens"),
		common.BytesToHash(adjustment.ValueTokens.Bytes()))

	// Deviation
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash(prefix+"deviation"),
		common.BytesToHash(adjustment.DeviationBps.Bytes()))

	// New supply
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash(prefix+"new_supply"),
		common.BytesToHash(adjustment.NewSupply.Bytes()))

	// Timestamp
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash(prefix+"timestamp"),
		common.BytesToHash(big.NewInt(adjustment.Timestamp.Unix()).Bytes()))

	log.Debug("Updated adjustment history", "index", count.String())
}

// SubscribeToUpdates subscribes to UltraStable token updates
func (m *UltraStableManager) SubscribeToUpdates(ch chan<- seigniorage.AdjustmentResult) event.Subscription {
	return m.scope.Track(m.updateFeed.Subscribe(ch))
}

// SubscribeToAdjustments subscribes to supply adjustment events
func (m *UltraStableManager) SubscribeToAdjustments(ch chan<- seigniorage.AdjustmentResult) event.Subscription {
	return m.scope.Track(m.adjustFeed.Subscribe(ch))
}

// GetStableConfig returns the UltraStable token configuration
func (m *UltraStableManager) GetStableConfig() *ultrastable.Config {
	return m.proprietary.GetStableConfig()
}

// GetCurrentStableValue returns the current market value of the UltraStable token
func (m *UltraStableManager) GetCurrentStableValue() *big.Int {
	return m.proprietary.GetCurrentStableValue()
}

// GetTargetStableValue returns the target oracle value of the UltraStable token
func (m *UltraStableManager) GetTargetStableValue() *big.Int {
	return m.proprietary.GetTargetStableValue()
}

// ForceUpdate triggers an immediate update from the oracle
func (m *UltraStableManager) ForceUpdate(ctx context.Context) error {
	// Query oracle for latest data
	if err := m.proprietary.QueryAIOracle(ctx); err != nil {
		return err
	}

	// Process the update
	m.ProcessUpdate()

	return nil
}

// GetVolatilityReduction returns the estimated volatility reduction factor
func (m *UltraStableManager) GetVolatilityReduction() float64 {
	return m.proprietary.GetVolatilityReduction()
}

// UpdateMarketValue updates the current market value of the UltraStable token
func (m *UltraStableManager) UpdateMarketValue(value *big.Int) {
	m.proprietary.SetCurrentStableValue(value)

	// Store in state
	statedb, err := m.blockchain.State()
	if err != nil {
		log.Error("Failed to get state for market value update", "error", err)
		return
	}

	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_current_value"),
		common.BytesToHash(value.Bytes()))

	log.Info("Updated UltraStable market value", "value", value)
}

// GetAdjustmentHistory returns recent adjustment history
func (m *UltraStableManager) GetAdjustmentHistory(maxEntries int) []seigniorage.AdjustmentResult {
	statedb, err := m.blockchain.State()
	if err != nil {
		log.Error("Failed to get state for history retrieval", "error", err)
		return nil
	}

	// Get current adjustment count
	countBytes := statedb.GetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("adjustment_history_count"))
	count := new(big.Int).SetBytes(countBytes[:]).Int64()

	results := make([]seigniorage.AdjustmentResult, 0)

	// Determine range to fetch
	start := count - int64(maxEntries)
	if start < 0 {
		start = 0
	}

	// Fetch entries
	for i := start; i < count; i++ {
		prefix := "adjustment_" + big.NewInt(i).String() + "_"

		// Type
		typeBytes := statedb.GetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash(prefix+"type"))
		typeValue := new(big.Int).SetBytes(typeBytes[:]).Int64()

		var adjustType seigniorage.AdjustmentType
		switch typeValue {
		case 1:
			adjustType = seigniorage.Expansion
		case 2:
			adjustType = seigniorage.Contraction
		default:
			adjustType = seigniorage.None
		}

		// Amount
		amountBytes := statedb.GetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash(prefix+"amount"))
		amount := new(big.Int).SetBytes(amountBytes[:])

		// Value tokens
		valueTokensBytes := statedb.GetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash(prefix+"value_tokens"))
		valueTokens := new(big.Int).SetBytes(valueTokensBytes[:])

		// Deviation
		deviationBytes := statedb.GetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash(prefix+"deviation"))
		deviation := new(big.Int).SetBytes(deviationBytes[:])

		// New supply
		supplyBytes := statedb.GetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash(prefix+"new_supply"))
		newSupply := new(big.Int).SetBytes(supplyBytes[:])

		// Timestamp
		timestampBytes := statedb.GetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash(prefix+"timestamp"))
		timestamp := new(big.Int).SetBytes(timestampBytes[:]).Int64()

		// Create result
		result := seigniorage.AdjustmentResult{
			Type:         adjustType,
			Amount:       amount,
			ValueTokens:  valueTokens,
			DeviationBps: deviation,
			NewSupply:    newSupply,
			Timestamp:    time.Unix(timestamp, 0),
		}

		results = append(results, result)
	}

	return results
}
