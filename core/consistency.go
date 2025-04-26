package core

import (
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// ConsistencyLevel defines the desired consistency guarantee.
type ConsistencyLevel int

const (
	// Strong consistency (e.g., requires full dBFT, longer timeouts).
	Strong ConsistencyLevel = iota
	// Eventual consistency (e.g., might relax dBFT requirements, shorter timeouts, relies on later reconciliation).
	Eventual
	// Add other levels like Causal, ReadYourWrites if needed.
)

// String representation for logging.
func (cl ConsistencyLevel) String() string {
	switch cl {
	case Strong:
		return "Strong"
	case Eventual:
		return "Eventual"
	default:
		return "Unknown"
	}
}

// ConsistencyConfig holds thresholds for switching levels.
type ConsistencyConfig struct {
	LatencyThresholdStrongMs  int64   // If avg latency exceeds this, consider moving away from Strong
	PartitionThresholdStrong  bool    // If partition detected, move away from Strong
	PacketLossThresholdStrong float64 // If packet loss exceeds this, move away from Strong

	// Add thresholds for switching back to Strong if needed
	LatencyThresholdEventualMs  int64
	PacketLossThresholdEventual float64
}

// DefaultConsistencyConfig provides default thresholds.
func DefaultConsistencyConfig() ConsistencyConfig {
	return ConsistencyConfig{
		LatencyThresholdStrongMs:  300,  // Move from Strong if latency > 300ms
		PartitionThresholdStrong:  true, // Move from Strong if partition detected
		PacketLossThresholdStrong: 5.0,  // Move from Strong if loss > 5%

		LatencyThresholdEventualMs:  100, // Consider Strong if latency < 100ms
		PacketLossThresholdEventual: 1.0, // Consider Strong if loss < 1%
	}
}

// ConsistencyOrchestrator monitors network conditions and adjusts the consistency level.
type ConsistencyOrchestrator struct {
	currentLevel   atomic.Value // Stores current ConsistencyLevel
	telemetry      *NetworkTelemetryMonitor
	config         ConsistencyConfig
	mu             sync.RWMutex // Protects config if mutable, or for complex logic
	stopChan       chan struct{}
	orchestratorWg sync.WaitGroup
	updateInterval time.Duration
}

// NewConsistencyOrchestrator creates and starts a consistency orchestrator.
func NewConsistencyOrchestrator(telemetry *NetworkTelemetryMonitor, config ConsistencyConfig, interval time.Duration) *ConsistencyOrchestrator {
	if telemetry == nil {
		log.Println("Warning: ConsistencyOrchestrator created without Telemetry Monitor.")
		// Or return an error
	}
	orchestrator := &ConsistencyOrchestrator{
		telemetry:      telemetry,
		config:         config,
		stopChan:       make(chan struct{}),
		updateInterval: interval,
	}
	// Start with Strong consistency by default
	orchestrator.currentLevel.Store(Strong)
	return orchestrator
}

// Start begins the orchestration loop.
func (co *ConsistencyOrchestrator) Start() {
	if co.telemetry == nil || co.updateInterval <= 0 {
		log.Println("[Consistency] Orchestrator disabled (no telemetry or interval <= 0).")
		return
	}
	log.Println("[Consistency] Starting orchestrator loop...")
	co.orchestratorWg.Add(1)
	go func() {
		defer co.orchestratorWg.Done()
		ticker := time.NewTicker(co.updateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-co.stopChan:
				log.Println("[Consistency] Stopping orchestrator loop.")
				return
			case <-ticker.C:
				co.evaluateAndUpdateLevel()
			}
		}
	}()
}

// Stop halts the orchestration loop.
func (co *ConsistencyOrchestrator) Stop() {
	// Avoid closing closed channel
	select {
	case <-co.stopChan:
		return
	default:
		close(co.stopChan)
	}
	co.orchestratorWg.Wait()
	log.Println("[Consistency] Orchestrator loop stopped.")
}

// evaluateAndUpdateLevel checks telemetry and updates the consistency level.
func (co *ConsistencyOrchestrator) evaluateAndUpdateLevel() {
	conditions := co.telemetry.GetCurrentConditions()
	currentLvl := co.GetCurrentLevel()
	newLvl := currentLvl // Assume no change initially

	// --- Logic to potentially move AWAY from Strong ---
	if currentLvl == Strong {
		if conditions.PartitionDetected && co.config.PartitionThresholdStrong {
			log.Printf("[Consistency] Partition detected, switching from Strong to Eventual.")
			newLvl = Eventual
		} else if conditions.AverageLatencyMs > co.config.LatencyThresholdStrongMs {
			log.Printf("[Consistency] High latency (%dms > %dms), switching from Strong to Eventual.",
				conditions.AverageLatencyMs, co.config.LatencyThresholdStrongMs)
			newLvl = Eventual
		} else if conditions.PacketLossPercent > co.config.PacketLossThresholdStrong {
			log.Printf("[Consistency] High packet loss (%.2f%% > %.2f%%), switching from Strong to Eventual.",
				conditions.PacketLossPercent, co.config.PacketLossThresholdStrong)
			newLvl = Eventual
		}
	}

	// --- Logic to potentially move back TO Strong ---
	if currentLvl == Eventual {
		// Require multiple conditions to be good before switching back?
		stableNetwork := !conditions.PartitionDetected &&
			conditions.AverageLatencyMs < co.config.LatencyThresholdEventualMs &&
			conditions.PacketLossPercent < co.config.PacketLossThresholdEventual

		if stableNetwork {
			log.Printf("[Consistency] Network stable (Latency %dms, Loss %.2f%%, No Partition), switching back to Strong.",
				conditions.AverageLatencyMs, conditions.PacketLossPercent)
			newLvl = Strong
		}
	}

	// Update if changed
	if newLvl != currentLvl {
		co.currentLevel.Store(newLvl)
		log.Printf("[Consistency] Level changed from %s to %s", currentLvl, newLvl)
	}
}

// GetCurrentLevel returns the current consistency level.
func (co *ConsistencyOrchestrator) GetCurrentLevel() ConsistencyLevel {
	level, ok := co.currentLevel.Load().(ConsistencyLevel)
	if !ok {
		// Should not happen
		log.Println("Error: Failed to load consistency level, returning Strong default.")
		return Strong
	}
	return level
}

// GetAdaptiveTimeout adjusts a base timeout based on current network conditions and consistency level.
func (co *ConsistencyOrchestrator) GetAdaptiveTimeout(baseTimeout time.Duration) time.Duration {
	level := co.GetCurrentLevel()
	conditions := co.telemetry.GetCurrentConditions()

	// Simple adaptive logic: increase timeout with latency, maybe more in Strong mode.
	adaptiveTimeout := baseTimeout + (time.Duration(conditions.AverageLatencyMs) * time.Millisecond)

	// If partition detected or high loss, increase significantly, especially for Strong
	if conditions.PartitionDetected || conditions.PacketLossPercent > 10.0 {
		adaptiveTimeout *= 2
	}

	// Apply different base multipliers based on level?
	if level == Strong {
		// Ensure timeout is reasonably high for Strong
		adaptiveTimeout = time.Duration(float64(adaptiveTimeout) * 1.5)
	} else { // Eventual
		// Allow shorter timeouts potentially
		adaptiveTimeout = time.Duration(float64(adaptiveTimeout) * 0.8)
	}

	// Set min/max bounds
	minTimeout := 500 * time.Millisecond // Example min
	maxTimeout := 30 * time.Second       // Example max
	if adaptiveTimeout < minTimeout {
		adaptiveTimeout = minTimeout
	}
	if adaptiveTimeout > maxTimeout {
		adaptiveTimeout = maxTimeout
	}

	// Log occasionally for debugging
	if rand.Intn(10) == 0 {
		log.Printf("[Consistency] Adaptive Timeout: Base=%v, Latency=%dms, Level=%s -> Adaptive=%v",
			baseTimeout, conditions.AverageLatencyMs, level, adaptiveTimeout)
	}

	return adaptiveTimeout
}

// GetAdaptiveRetries determines the number of retries based on conditions.
// (Placeholder - more complex logic needed based on operation type)
func (co *ConsistencyOrchestrator) GetAdaptiveRetries() int {
	level := co.GetCurrentLevel()
	conditions := co.telemetry.GetCurrentConditions()
	retries := 1 // Default retries

	if conditions.PacketLossPercent > 5.0 {
		retries = 2
	}
	if conditions.PartitionDetected {
		retries = 3
	}

	// Fewer retries might be acceptable in Eventual mode for some operations
	if level == Eventual && retries > 1 {
		retries--
	}

	return retries
}
