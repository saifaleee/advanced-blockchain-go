package core

import (
	"fmt"
	"log"
	"math"
	"sync"
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
		if v.IsActive.Load() && v.Node.IsAuthenticated.Load() && v.Reputation.Load() >= minReputationToParticipate {
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
func (vm *ValidatorManager) SimulateDBFT(proposedBlock *Block, proposer *Validator) (bool, []NodeID) {
	shardID := proposedBlock.Header.ShardID
	log.Printf("Shard %d: Starting simulated dBFT for block H:%d proposed by %s",
		shardID, proposedBlock.Header.Height, proposer.Node.ID) // Use proposer.Node.ID safely

	validators := vm.GetActiveValidatorsForShard(shardID)
	if len(validators) == 0 {
		log.Printf("Shard %d: dBFT failed - No eligible validators found.", shardID)
		if proposer != nil && proposer.Node != nil { // Check proposer validity
			// Check if proposer exists in manager before updating rep
			if actualProposer, exists := vm.GetValidator(proposer.Node.ID); exists {
				actualProposer.UpdateReputation(DbftConsensusPenalty) // Penalize proposer
			} else {
				log.Printf("Warning: Proposer %s not found in manager during penalty application.", proposer.Node.ID)
			}
		}
		return false, nil
	}

	requiredVotes := int(math.Floor(float64(len(validators))*float64(dBFTThresholdNumerator)/float64(dBFTThresholdDenominator))) + 1
	log.Printf("Shard %d: dBFT requires %d votes out of %d eligible validators.", shardID, requiredVotes, len(validators))

	agreementVotes := 0
	agreeingValidators := make([]NodeID, 0, len(validators))
	pow := NewProofOfWork(proposedBlock)

	for _, v := range validators {
		// Validate the block proposal (PoW check is the main part here)
		isValid := pow.Validate()
		// Add other validation checks here in a real system (state root, tx validity etc.)

		if isValid {
			agreementVotes++
			agreeingValidators = append(agreeingValidators, v.Node.ID)
			// log.Printf("Shard %d: Validator %s votes YES (Block H:%d Valid)", shardID, v.Node.ID, proposedBlock.Header.Height)
		} else {
			// Validator correctly identifies an invalid block - NO PENALTY for voting NO here.
			log.Printf("Shard %d: Validator %s votes NO (Block H:%d INVALID PoW/Structure)", shardID, v.Node.ID, proposedBlock.Header.Height)
			// *** REMOVED PENALTY FOR VOTING NO ***
			// v.UpdateReputation(DbftConsensusPenalty)
		}
	}

	if agreementVotes >= requiredVotes {
		log.Printf("Shard %d: dBFT Consensus REACHED for Block H:%d (%d/%d votes >= %d required)",
			shardID, proposedBlock.Header.Height, agreementVotes, len(validators), requiredVotes)

		// Reward proposer (if they exist in the manager)
		if proposer != nil && proposer.Node != nil {
			if actualProposer, exists := vm.GetValidator(proposer.Node.ID); exists {
				actualProposer.UpdateReputation(DbftConsensusReward)
			}
		}
		// Reward agreeing validators
		for _, agreeID := range agreeingValidators {
			if val, ok := vm.GetValidator(agreeID); ok {
				// Avoid double reward if proposer also voted yes
				if proposer == nil || proposer.Node == nil || val.Node.ID != proposer.Node.ID {
					val.UpdateReputation(DbftConsensusReward)
				}
			}
		}
		return true, agreeingValidators
	}

	// Consensus failed
	log.Printf("Shard %d: dBFT Consensus FAILED for Block H:%d (%d/%d votes < %d required)",
		shardID, proposedBlock.Header.Height, agreementVotes, len(validators), requiredVotes)

	// Penalize proposer ONLY if consensus failed
	if proposer != nil && proposer.Node != nil {
		if actualProposer, exists := vm.GetValidator(proposer.Node.ID); exists {
			actualProposer.UpdateReputation(DbftConsensusPenalty) // Penalize proposer
		} else {
			log.Printf("Warning: Proposer %s not found in manager during consensus failure penalty application.", proposer.Node.ID)
		}
	}

	return false, nil
}
