package core

import (
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// NetworkConditions represents simulated network health metrics.
type NetworkConditions struct {
	AverageLatencyMs  int64   // Simulated average peer-to-peer latency
	PartitionDetected bool    // Simulated flag indicating a network partition
	PacketLossPercent float64 // Simulated packet loss percentage
}

// NetworkTelemetryMonitor simulates monitoring network conditions.
type NetworkTelemetryMonitor struct {
	currentConditions atomic.Value // Holds the current *NetworkConditions
	stopChan          chan struct{}
	monitorWg         sync.WaitGroup
	updateInterval    time.Duration
}

// NewNetworkTelemetryMonitor creates and starts a simulated monitor.
func NewNetworkTelemetryMonitor(interval time.Duration) *NetworkTelemetryMonitor {
	monitor := &NetworkTelemetryMonitor{
		stopChan:       make(chan struct{}),
		updateInterval: interval,
	}
	// Initialize with default stable conditions
	monitor.currentConditions.Store(&NetworkConditions{
		AverageLatencyMs:  50, // Stable latency
		PartitionDetected: false,
		PacketLossPercent: 0.5, // Low packet loss
	})
	return monitor
}

// Start begins the monitoring simulation loop.
func (m *NetworkTelemetryMonitor) Start() {
	if m.updateInterval <= 0 {
		log.Println("[Telemetry] Monitor disabled (interval <= 0).")
		return
	}
	log.Println("[Telemetry] Starting network condition simulator...")
	m.monitorWg.Add(1)
	go func() {
		defer m.monitorWg.Done()
		ticker := time.NewTicker(m.updateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopChan:
				log.Println("[Telemetry] Stopping network condition simulator.")
				return
			case <-ticker.C:
				m.simulateUpdate()
			}
		}
	}()
}

// Stop halts the monitoring simulation.
func (m *NetworkTelemetryMonitor) Stop() {
	// Avoid closing closed channel
	select {
	case <-m.stopChan:
		// Already closed or closing
		return
	default:
		// Only close if not already closed
		close(m.stopChan)
	}
	m.monitorWg.Wait()
	log.Println("[Telemetry] Network condition simulator stopped.")
}

// simulateUpdate generates new simulated network conditions.
func (m *NetworkTelemetryMonitor) simulateUpdate() {
	// Simulate fluctuating conditions randomly
	latency := int64(50 + rand.Intn(250)) // Base 50ms + up to 250ms variance
	partition := rand.Float32() < 0.05    // 5% chance of simulated partition
	packetLoss := rand.Float64() * 5.0    // 0-5% packet loss

	if partition {
		// Partitions often correlate with higher latency and loss
		latency += int64(rand.Intn(500))
		packetLoss = 5.0 + rand.Float64()*10.0 // Higher loss during partition
		if packetLoss > 100.0 {
			packetLoss = 100.0
		}
	}

	newConditions := &NetworkConditions{
		AverageLatencyMs:  latency,
		PartitionDetected: partition,
		PacketLossPercent: packetLoss,
	}
	m.currentConditions.Store(newConditions)
	// Log less frequently to avoid spamming
	if rand.Intn(5) == 0 {
		log.Printf("[Telemetry] Simulated Conditions - Latency: %dms, Partition: %t, Loss: %.2f%%",
			latency, partition, packetLoss)
	}
}

// GetCurrentConditions returns the latest simulated network conditions.
func (m *NetworkTelemetryMonitor) GetCurrentConditions() *NetworkConditions {
	// The Load method returns an interface{}, so we need to type assert it.
	conditions, ok := m.currentConditions.Load().(*NetworkConditions)
	if !ok || conditions == nil {
		// This should ideally not happen if initialized correctly
		log.Println("Error: Failed to load network conditions, returning default.")
		// Return a default non-nil value to prevent nil pointer errors downstream
		return &NetworkConditions{AverageLatencyMs: 100, PartitionDetected: false, PacketLossPercent: 1.0}
	}
	return conditions
}
