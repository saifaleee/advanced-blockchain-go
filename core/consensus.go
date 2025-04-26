package core

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

const (
	minReputationToParticipate = 0
	dBFTThresholdNumerator     = 2
	dBFTThresholdDenominator   = 3
	// Exported Constants: Capitalize first letter
	DbftConsensusReward  = 1  // Reward for successful proposal/validation
	DbftConsensusPenalty = -2 // Penalty for failed proposal/validation
)

// ValidatorManager manages the set of active validators.
type ValidatorManager struct {
	validators map[NodeID]*Validator
	mu         sync.RWMutex // Protects the validators map
}

// NewValidatorManager creates a new validator manager.
func NewValidatorManager() *ValidatorManager {
	return &ValidatorManager{
		validators: make(map[NodeID]*Validator),
	}
}

// AddValidator adds a node to the validator set.
func (vm *ValidatorManager) AddValidator(node *Node, initialReputation int64) error {
	if node == nil {
		return fmt.Errorf("cannot add nil node as validator")
	}
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if _, exists := vm.validators[node.ID]; exists {
		return fmt.Errorf("validator %s already exists", node.ID)
	}
	if !node.IsAuthenticated.Load() {
		log.Printf("Warning: Adding validator %s who is not yet authenticated.", node.ID)
		// return fmt.Errorf("node %s must be authenticated to become a validator", node.ID)
	}

	validator := NewValidator(node, initialReputation)
	vm.validators[node.ID] = validator
	log.Printf("Validator %s added with reputation %d", node.ID, initialReputation)
	return nil
}

// GetValidator retrieves a validator by ID.
func (vm *ValidatorManager) GetValidator(id NodeID) (*Validator, bool) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	v, ok := vm.validators[id]
	return v, ok
}

// GetAllValidators returns a slice of all current validators.
func (vm *ValidatorManager) GetAllValidators() []*Validator {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	list := make([]*Validator, 0, len(vm.validators))
	for _, v := range vm.validators {
		list = append(list, v)
	}
	return list
}

// GetActiveValidatorsForShard returns the list of validators eligible for consensus for a given shard.
// In this simulation, we return all active, authenticated validators with sufficient reputation, regardless of shard.
// A real implementation would likely have shard-specific assignments.
func (vm *ValidatorManager) GetActiveValidatorsForShard(shardID uint64) []*Validator {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	eligibleValidators := make([]*Validator, 0, len(vm.validators))
	for _, v := range vm.validators {
		// Ensure node is not nil before accessing fields
		if v.Node != nil && v.IsActive.Load() && v.Node.IsAuthenticated.Load() && v.Reputation.Load() >= minReputationToParticipate {
			eligibleValidators = append(eligibleValidators, v)
		} else {
			// log.Printf("Validator %s skipped for shard %d consensus (Active: %v, Auth: %v, Rep: %d)",
			//  v.Node.ID, shardID, v.IsActive.Load(), v.Node.IsAuthenticated.Load(), v.Reputation.Load())
		}
	}
	return eligibleValidators
}

// SimulateDBFT performs a simulated dBFT consensus round for a proposed block.
// Returns true if consensus is reached, and the list of agreeing validator IDs.
// Modify function signature:
func (vm *ValidatorManager) SimulateDBFT(
	proposedBlock *Block,
	proposer *Validator, // Proposer info (can be temporary if node not in manager)
	level ConsistencyLevel, // <<< ADDED parameter
	co *ConsistencyOrchestrator, // <<< ADDED parameter
) (bool, []NodeID) {

	// Basic nil checks
	if proposedBlock == nil || proposedBlock.Header == nil {
		log.Printf("Error: SimulateDBFT called with nil block or header.")
		return false, nil
	}
	if proposer == nil || proposer.Node == nil {
		log.Printf("Error: SimulateDBFT called with nil proposer or proposer node.")
		// Cannot penalize proposer if info is missing
		return false, nil
	}
	if co == nil || co.telemetry == nil {
		log.Printf("Error: SimulateDBFT called with nil ConsistencyOrchestrator or Telemetry.")
		// Cannot get adaptive timeout or conditions, proceed with defaults? Or fail? Fail safer.
		return false, nil
	}

	shardID := proposedBlock.Header.ShardID
	log.Printf("Shard %d: Starting simulated dBFT (%s) for block H:%d proposed by %s",
		shardID, level, proposedBlock.Header.Height, proposer.Node.ID) // Log level

	// --- Adaptive Timeout (Conceptual) ---
	// In a real network scenario, you'd use this timeout for communication rounds.
	// Here, we just calculate and log it.
	baseTimeout := 5 * time.Second // Example base timeout for dBFT round
	adaptiveTimeout := co.GetAdaptiveTimeout(baseTimeout)
	currentConditions := co.telemetry.GetCurrentConditions() // Get conditions once
	log.Printf("Shard %d: Using adaptive dBFT timeout: %v (based on Level: %s, Latency: %dms)",
		shardID, adaptiveTimeout, level, currentConditions.AverageLatencyMs)
	// --- End Adaptive Timeout ---

	validators := vm.GetActiveValidatorsForShard(shardID)
	if len(validators) == 0 {
		log.Printf("Shard %d: dBFT failed - No eligible validators found.", shardID)
		// Penalize the proposer if they exist in the manager
		if actualProposer, exists := vm.GetValidator(proposer.Node.ID); exists {
			actualProposer.UpdateReputation(DbftConsensusPenalty) // Penalize proposer
		} else {
			log.Printf("Warning: Proposer %s not found in manager during penalty application (no validators).", proposer.Node.ID)
		}
		return false, nil
	}

	// --- Adaptive Threshold (Example) ---
	// We could potentially relax the threshold for Eventual consistency,
	// but this weakens BFT guarantees significantly. Use with extreme caution!
	// For this PoC, we'll keep the standard threshold but log the possibility.
	requiredVotes := int(math.Floor(float64(len(validators))*float64(dBFTThresholdNumerator)/float64(dBFTThresholdDenominator))) + 1

	if level == Eventual {
		// Example: Could potentially lower requiredVotes here, but NOT recommended for BFT.
		log.Printf("Shard %d: Operating under Eventual consistency. (Standard BFT threshold %d/%d still applied for simulation)",
			shardID, dBFTThresholdNumerator, dBFTThresholdDenominator)
	}
	// --- End Adaptive Threshold ---

	log.Printf("Shard %d: dBFT requires %d votes out of %d eligible validators.", shardID, requiredVotes, len(validators))

	agreementVotes := 0
	agreeingValidators := make([]NodeID, 0, len(validators))
	pow := NewProofOfWork(proposedBlock)

	// --- Simulate Voting (with conceptual timeout) ---
	// In reality, you'd collect votes within the 'adaptiveTimeout' duration.
	// The core validation logic remains the same for this simulation.
	for _, v := range validators {
		// Ensure validator node is not nil
		if v.Node == nil {
			log.Printf("Warning: Skipping validator with nil Node during voting.")
			continue
		}
		isValid := pow.Validate() // Simple validation check
		// TODO: Add state root validation, transaction validation etc.

		if isValid {
			agreementVotes++
			agreeingValidators = append(agreeingValidators, v.Node.ID)
		} else {
			log.Printf("Shard %d: Validator %s votes NO (Block H:%d INVALID PoW/Structure)", shardID, v.Node.ID, proposedBlock.Header.Height)
			// No penalty for voting NO on an invalid block
		}
	}
	// --- End Simulate Voting ---

	if agreementVotes >= requiredVotes {
		log.Printf("Shard %d: dBFT Consensus REACHED for Block H:%d (%d/%d votes >= %d required)",
			shardID, proposedBlock.Header.Height, agreementVotes, len(validators), requiredVotes)

		// Reward proposer (only if they are a registered validator in the manager)
		if actualProposer, exists := vm.GetValidator(proposer.Node.ID); exists {
			actualProposer.UpdateReputation(DbftConsensusReward)
		} // No reward/penalty if proposer wasn't in the manager

		// Reward agreeing validators
		for _, agreeID := range agreeingValidators {
			if val, ok := vm.GetValidator(agreeID); ok {
				// Avoid double reward if proposer also voted yes AND was a registered validator
				if val.Node.ID != proposer.Node.ID || !vm.isRegisteredValidator(proposer.Node.ID) {
					val.UpdateReputation(DbftConsensusReward)
				}
			}
		}
		return true, agreeingValidators
	}

	// Consensus failed
	log.Printf("Shard %d: dBFT Consensus FAILED (%s) for Block H:%d (%d/%d votes < %d required)",
		shardID, level, proposedBlock.Header.Height, agreementVotes, len(validators), requiredVotes) // Log level

	// Penalize proposer ONLY if consensus failed (and if they are a registered validator)
	if actualProposer, exists := vm.GetValidator(proposer.Node.ID); exists {
		actualProposer.UpdateReputation(DbftConsensusPenalty) // Penalize proposer
	} else {
		log.Printf("Warning: Proposer %s not found in manager during consensus failure penalty application.", proposer.Node.ID)
	}

	return false, nil
}

// isRegisteredValidator is a helper to check if a node ID exists in the manager (read-locked).
func (vm *ValidatorManager) isRegisteredValidator(id NodeID) bool {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	_, exists := vm.validators[id]
	return exists
}
