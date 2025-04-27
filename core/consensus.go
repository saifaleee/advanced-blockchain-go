package core

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log"
	"math/big"
	mrand "math/rand" // Alias math/rand to avoid conflict with crypto/rand
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	minReputationToParticipate = 0
	dBFTThresholdNumerator     = 2
	dBFTThresholdDenominator   = 3
	DbftConsensusReward        = 1               // Reward for successful proposal/validation
	DbftConsensusPenalty       = -2              // Penalty for failed proposal/validation
	challengeTimeout           = 1 * time.Minute // How long a validator has to respond
)

type Signature []byte // Represents a cryptographic signature

type SignedBlock struct {
	Block            *Block
	FinalityVotes    map[NodeID]Signature // Map Validator ID to their actual signature
	ConsensusReached bool
}

// PendingChallenge stores information about an ongoing challenge.
type PendingChallenge struct {
	ChallengeData []byte
	ExpectedNonce uint64 // Nonce the validator should use
	Expiry        time.Time
}

// ValidatorManager manages the set of validators and their reputations.
type ValidatorManager struct {
	Validators        map[NodeID]*Validator
	ReputationScores  map[NodeID]int64 // Store reputation scores separately for easier access
	mu                sync.RWMutex
	randSource        *mrand.Rand // Use math/rand for non-crypto randomness
	PendingChallenges map[NodeID]PendingChallenge
	ChallengeMu       sync.Mutex // Lock specifically for challenges
}

// NewValidatorManager creates a new validator manager.
func NewValidatorManager() *ValidatorManager {
	return &ValidatorManager{
		Validators:        make(map[NodeID]*Validator),
		ReputationScores:  make(map[NodeID]int64),
		randSource:        mrand.New(mrand.NewSource(time.Now().UnixNano())), // Seed math/rand
		PendingChallenges: make(map[NodeID]PendingChallenge),
	}
}

// AddValidator adds a new validator to the manager.
func (vm *ValidatorManager) AddValidator(node *Node, initialReputation int64) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	if _, exists := vm.Validators[node.ID]; exists {
		log.Printf("Validator %s already exists.", node.ID)
		return
	}
	validator := &Validator{
		Node:       node,
		Reputation: atomic.Int64{}, // Use atomic.Int64 as a value, not a pointer
		IsActive:   atomic.Bool{},  // Replace AtomicBool with atomic.Bool
	}
	validator.Reputation.Store(initialReputation)
	validator.IsActive.Store(true) // Assume active initially
	vm.Validators[node.ID] = validator
	vm.ReputationScores[node.ID] = initialReputation // Initialize score in the map
	log.Printf("Added Validator: %s with Reputation: %d", node.ID, initialReputation)
}

// GetValidator retrieves a validator by ID.
func (vm *ValidatorManager) GetValidator(id NodeID) (*Validator, bool) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	v, ok := vm.Validators[id]
	return v, ok
}

// GetAllValidators returns a slice of all registered validators.
func (vm *ValidatorManager) GetAllValidators() []*Validator {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	validators := make([]*Validator, 0, len(vm.Validators))
	for _, v := range vm.Validators {
		validators = append(validators, v)
	}
	return validators
}

// UpdateReputation adjusts the reputation score for a validator.
func (vm *ValidatorManager) UpdateReputation(id NodeID, change int64) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	if v, ok := vm.Validators[id]; ok {
		currentRep := v.Reputation.Load()
		newRep := currentRep + change
		if newRep < 1 { // Ensure reputation doesn't fall below 1
			newRep = 1
		}
		v.Reputation.Store(newRep)
		vm.ReputationScores[id] = newRep // Update score in the map

		// Add slashing conditions for adversarial behavior
		if change < 0 {
			log.Printf("[Slashing] Validator %s penalized by %d points.", id, -change)
			if newRep < 10 { // Threshold for slashing
				log.Printf("[Slashing] Validator %s reputation critically low (%d). Marking as inactive.", id, newRep)
				v.IsActive.Store(false)
			}
		}
	} else {
		log.Printf("Attempted to update reputation for non-existent validator %s", id)
	}
}

// GetReputation retrieves the reputation score for a validator.
func (vm *ValidatorManager) GetReputation(id NodeID) (int64, bool) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	if v, ok := vm.Validators[id]; ok {
		return v.Reputation.Load(), true
	}
	return 0, false
}

// LogReputations prints the current reputation scores of all validators.
func (vm *ValidatorManager) LogReputations() {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	log.Println("--- Current Validator Reputations ---")
	// Sort by NodeID for consistent output
	ids := make([]NodeID, 0, len(vm.Validators))
	for id := range vm.Validators {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		v := vm.Validators[id]
		log.Printf("  Validator %s: Reputation %d (Active: %t, Authenticated: %t)",
			id, v.Reputation.Load(), v.IsActive.Load(), v.Node.IsAuthenticated())
	}
	log.Println("-------------------------------------")
}

// GetActiveValidatorsForShard returns validators considered active and eligible for consensus on a shard.
// Currently global, but could be adapted for per-shard assignment.
// Now also checks Node.IsAuthenticated status.
func (vm *ValidatorManager) GetActiveValidatorsForShard(shardID uint64) []*Validator {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	active := make([]*Validator, 0)
	for _, v := range vm.Validators {
		// Check IsActive status AND IsAuthenticated status
		if v.IsActive.Load() && v.Node.IsAuthenticated() { // Correct usage of IsAuthenticated as a function
			active = append(active, v)
		}
	}
	return active
}

// FinalizeBlockDBFT simulates the dBFT consensus process for finalizing a proposed block.
// Uses actual (simulated) ECDSA signatures for voting.
// Implements adaptive threshold based on reputation.
func (vm *ValidatorManager) FinalizeBlockDBFT(block *Block, shardID uint64) (*SignedBlock, error) {
	vm.mu.RLock()

	eligibleValidators := vm.GetActiveValidatorsForShard(shardID)
	if len(eligibleValidators) == 0 {
		vm.mu.RUnlock()
		return nil, fmt.Errorf("no eligible validators for shard %d", shardID)
	}

	// Calculate adaptive threshold based on total reputation
	thresholdReputation := vm.calculateAdaptiveThreshold(eligibleValidators)
	totalEligibleReputation := int64(0)
	for _, v := range eligibleValidators {
		totalEligibleReputation += v.Reputation.Load()
	}

	log.Printf("[Consensus] Shard %d: Starting dBFT for Block H:%d Hash:%x... Need > %.2f reputation (out of %d total from %d validators).",
		shardID, block.Header.Height, safeSlice(block.Hash, 4), float64(thresholdReputation), totalEligibleReputation, len(eligibleValidators))

	vm.mu.RUnlock()

	// Simulate voting process with signatures
	votesReputation := int64(0)
	agreeingSignatures := make(map[NodeID]Signature)
	var voteMu sync.Mutex

	dataToSign := block.Hash
	hashToSign := sha256.Sum256(dataToSign)

	var wg sync.WaitGroup
	for _, validator := range eligibleValidators {
		wg.Add(1)
		go func(v *Validator) {
			defer wg.Done()
			powInstance := NewProofOfWork(block)
			if !powInstance.Validate() {
				log.Printf("[Consensus] Shard %d: Validator %s found block invalid (PoW). Voting NO.", shardID, v.Node.ID)
				return
			}

			if vm.randSource.Float32() < 0.98 {
				signature, err := v.Node.SignData(hashToSign[:])
				if err != nil {
					log.Printf("[Consensus] Shard %d: Validator %s FAILED to sign block %x: %v", shardID, v.Node.ID, safeSlice(block.Hash, 4), err)
					return
				}

				time.Sleep(time.Duration(10+vm.randSource.Intn(50)) * time.Millisecond)

				isValidSig := ecdsa.VerifyASN1(v.Node.PublicKey, hashToSign[:], signature) // Remove parentheses from PublicKey
				if isValidSig {
					voteMu.Lock()
					agreeingSignatures[v.Node.ID] = signature
					votesReputation += v.Reputation.Load()
					voteMu.Unlock()
				} else {
					log.Printf("[Consensus] Shard %d: Validator %s produced INVALID signature for block %x!", shardID, v.Node.ID, safeSlice(block.Hash, 4))
					// Add Penalty for invalid signature (Ticket 6)
					voteMu.Lock()
					vm.UpdateReputation(v.Node.ID, -5) // Penalize for invalid signature
					v.Node.UpdateTrustScore(-0.1)      // Decrease trust score
					voteMu.Unlock()
				}
			} else {
				log.Printf("[Consensus] Shard %d: Validator %s (simulated) voted NO for Block %x", shardID, v.Node.ID, safeSlice(block.Hash, 4))
			}
		}(validator)
	}
	wg.Wait()

	consensusReached := votesReputation > thresholdReputation

	signedBlock := &SignedBlock{
		Block:            block,
		FinalityVotes:    agreeingSignatures,
		ConsensusReached: consensusReached,
	}

	go func(validators []*Validator, reached bool, agreeing map[NodeID]Signature) {
		vm.mu.Lock()
		defer vm.mu.Unlock()

		for _, v := range validators {
			_, agreed := agreeing[v.Node.ID]
			currentRep := vm.ReputationScores[v.Node.ID]
			change := int64(0)

			if reached {
				if agreed {
					change = 1
				} else {
					change = -2
				}
			} else {
				if !agreed {
					change = -1
				}
			}

			newRep := currentRep + change
			if newRep < 1 {
				newRep = 1
			}
			vm.ReputationScores[v.Node.ID] = newRep
			if val, ok := vm.Validators[v.Node.ID]; ok {
				val.Reputation.Store(newRep)
			}
		}
	}(eligibleValidators, consensusReached, agreeingSignatures)

	if consensusReached {
		log.Printf("[Consensus] Shard %d: dBFT Consensus REACHED for Block H:%d Hash:%x... (%d/%d Reputation)",
			shardID, block.Header.Height, safeSlice(block.Hash, 4), votesReputation, thresholdReputation)

		finalizerIDs := make([]NodeID, 0, len(agreeingSignatures))
		for id := range agreeingSignatures {
			finalizerIDs = append(finalizerIDs, id)
		}
		sort.Slice(finalizerIDs, func(i, j int) bool { return finalizerIDs[i] < finalizerIDs[j] })
		block.Header.FinalitySignatures = finalizerIDs

		return signedBlock, nil
	}

	log.Printf("[Consensus] Shard %d: dBFT Consensus FAILED for Block H:%d Hash:%x... (%d/%d Reputation)",
		shardID, block.Header.Height, safeSlice(block.Hash, 4), votesReputation, thresholdReputation)
	return signedBlock, fmt.Errorf("dBFT consensus failed for block %s on shard %d (%d/%d reputation)", block.Hash, shardID, votesReputation, thresholdReputation)
}

// calculateAdaptiveThreshold calculates the consensus threshold based on total reputation.
// Returns the minimum reputation required to reach consensus (> 2/3 of total eligible reputation).
func (vm *ValidatorManager) calculateAdaptiveThreshold(eligibleValidators []*Validator) int64 {
	if len(eligibleValidators) == 0 {
		return 0
	}
	totalReputation := int64(0)
	for _, v := range eligibleValidators {
		totalReputation += v.Reputation.Load()
	}

	if totalReputation == 0 {
		return 0
	}

	threshold := (totalReputation * 2 / 3)

	return threshold
}

// SelectDelegateWithVRF selects a delegate using a Verifiable Random Function (placeholder).
// Uses a combination of a seed (e.g., prev block hash) and shard ID for input.
func (vm *ValidatorManager) SelectDelegateWithVRF(seed []byte, shardID uint64, blockHeight uint64) (*Validator, error) { // Added blockHeight
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	eligible := vm.GetActiveValidatorsForShard(shardID)
	if len(eligible) == 0 {
		return nil, fmt.Errorf("no eligible validators for VRF selection on shard %d", shardID)
	}

	var bestValidator *Validator
	var lowestValue *big.Int

	log.Printf("[Consensus] Simulating VRF Delegate Selection for Shard %d (H:%d) with %d candidates.", shardID, blockHeight, len(eligible))

	// Create a more deterministic base input for this selection round
	baseInput := bytes.Join(
		[][]byte{
			seed, // e.g., previous block hash
			[]byte(fmt.Sprintf("%d", shardID)),
			[]byte(fmt.Sprintf("%d", blockHeight)), // Include height
		},
		[]byte{},
	)

	for _, v := range eligible {
		// Combine base input with validator-specific info
		vrfInput := append(append([]byte{}, baseInput...), []byte(v.Node.ID)...)
		hash := sha256.Sum256(vrfInput)
		value := new(big.Int).SetBytes(hash[:])

		// In a real VRF, we'd verify the proof here.
		// We assume the simulated generation is always valid for the placeholder.
		isValidProof := true

		if isValidProof {
			if lowestValue == nil || value.Cmp(lowestValue) < 0 {
				lowestValue = value
				bestValidator = v
			}
		}
	}

	if bestValidator == nil {
		// This should ideally not happen if there are eligible validators, but handle defensively.
		log.Printf("[Consensus] VRF selection failed unexpectedly for shard %d (H:%d), falling back to random.", shardID, blockHeight)
		return eligible[vm.randSource.Intn(len(eligible))], nil
	}

	log.Printf("[Consensus] VRF selected Validator %s for Shard %d (H:%d) (Value: %s...)", bestValidator.Node.ID, shardID, blockHeight, lowestValue.String()[:10])
	return bestValidator, nil
}

// --- Challenge-Response (Ticket 8) --- //

// ChallengeValidator initiates a challenge for a given validator.
// Returns the challenge data or an error.
func (vm *ValidatorManager) ChallengeValidator(nodeID NodeID) ([]byte, error) {
	vm.mu.RLock() // Lock validator map briefly to get node
	v, exists := vm.Validators[nodeID]
	vm.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("validator %s not found for challenge", nodeID)
	}

	vm.ChallengeMu.Lock() // Lock challenge map
	defer vm.ChallengeMu.Unlock()

	if _, ongoing := vm.PendingChallenges[nodeID]; ongoing {
		// Optional: Could check expiry and reissue if expired
		return nil, fmt.Errorf("challenge already pending for validator %s", nodeID)
	}

	// Generate challenge data (e.g., 32 random bytes)
	challengeData := make([]byte, 32)
	_, err := rand.Read(challengeData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate challenge data: %w", err)
	}

	// Store pending challenge
	challenge := PendingChallenge{
		ChallengeData: challengeData,
		ExpectedNonce: v.Node.GetAuthNonce(), // Expect response signed with current nonce
		Expiry:        time.Now().Add(challengeTimeout),
	}
	vm.PendingChallenges[nodeID] = challenge

	log.Printf("[Auth] Issued challenge to Validator %s (Nonce: %d). Data: %x", nodeID, challenge.ExpectedNonce, safeSlice(challengeData, 8))
	return challengeData, nil
}

// VerifyResponse verifies a validator's response to a challenge.
// responseData should be the signature of sha256(challengeData + nonce).
func (vm *ValidatorManager) VerifyResponse(nodeID NodeID, responseData Signature) error {
	vm.ChallengeMu.Lock() // Lock challenge map
	pending, exists := vm.PendingChallenges[nodeID]
	if exists {
		delete(vm.PendingChallenges, nodeID) // Remove challenge once processed
	}
	vm.ChallengeMu.Unlock()

	if !exists {
		return fmt.Errorf("no pending challenge found for validator %s to verify response", nodeID)
	}

	if time.Now().After(pending.Expiry) {
		log.Printf("[Auth] Challenge response from %s expired.", nodeID)
		vm.UpdateReputation(nodeID, -5) // Penalize for expired response
		vm.Validators[nodeID].Node.UpdateTrustScore(-0.15)
		return fmt.Errorf("challenge for %s expired", nodeID)
	}

	vm.mu.RLock() // Lock validator map briefly
	v, vExists := vm.Validators[nodeID]
	vm.mu.RUnlock()
	if !vExists {
		// Should not happen if challenge existed, but check anyway
		return fmt.Errorf("validator %s disappeared during challenge verification", nodeID)
	}

	// Construct the data that should have been signed
	hasher := sha256.New()
	hasher.Write(pending.ChallengeData)
	nonceBytes := new(big.Int).SetUint64(pending.ExpectedNonce).Bytes()
	hasher.Write(nonceBytes)
	expectedHash := hasher.Sum(nil)

	// Verify the signature
	if !v.Node.VerifySignature(expectedHash, responseData) {
		log.Printf("[Auth] Challenge response verification FAILED for %s. Expected Nonce: %d", nodeID, pending.ExpectedNonce)
		vm.UpdateReputation(nodeID, -10) // Penalize heavily for incorrect response
		vm.Validators[nodeID].Node.UpdateTrustScore(-0.25)
		return fmt.Errorf("invalid signature in challenge response for %s", nodeID)
	}

	// Success!
	log.Printf("[Auth] Challenge response verification SUCCESSFUL for %s.", nodeID)
	vm.UpdateReputation(nodeID, 2)   // Reward for successful challenge response
	v.Node.LastAttested = time.Now() // Update last attested time
	v.Node.UpdateTrustScore(0.1)     // Increase trust
	// Ensure node is marked active if it passed a challenge
	if !v.IsActive.Load() {
		v.IsActive.Store(true)
		log.Printf("[Auth] Reactivated validator %s after successful challenge response.", nodeID)
	}

	// Increment node's nonce AFTER successful verification to prevent replay
	v.Node.IncrementAuthNonce()

	return nil
}

// CleanupExpiredChallenges removes challenges that have passed their expiry time.
func (vm *ValidatorManager) CleanupExpiredChallenges() {
	vm.ChallengeMu.Lock()
	defer vm.ChallengeMu.Unlock()

	now := time.Now()
	for nodeID, challenge := range vm.PendingChallenges {
		if now.After(challenge.Expiry) {
			log.Printf("[Auth] Cleaning up expired challenge for %s.", nodeID)
			delete(vm.PendingChallenges, nodeID)
			// Penalize validator for not responding in time
			// Use a separate goroutine to avoid holding the challenge lock during reputation update
			go vm.UpdateReputation(nodeID, -5)
			go func(id NodeID) {
				vm.mu.RLock()
				v, ok := vm.Validators[id]
				vm.mu.RUnlock()
				if ok {
					v.Node.UpdateTrustScore(-0.15)
				}
			}(nodeID)
		}
	}
}
