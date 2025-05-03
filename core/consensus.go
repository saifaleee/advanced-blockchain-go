package core

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary" // Added for nonce serialization
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
	challengeTimeout           = 1 * time.Second // Reduced timeout for testing expiry
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
		// Fixing GetActiveValidatorsForShard to filter active and authenticated validators
		if v.IsActive.Load() && v.Node.IsAuthenticated() {
			active = append(active, v)
		}
	}
	return active
}

// Refine FinalizeBlockDBFT to ensure consistent hash validation and reputation updates
func (vm *ValidatorManager) FinalizeBlockDBFT(block *Block, shardID uint64) (*SignedBlock, error) {
	vm.mu.RLock()

	eligibleValidators := vm.GetActiveValidatorsForShard(shardID)
	if len(eligibleValidators) == 0 {
		vm.mu.RUnlock()
		return nil, fmt.Errorf("no eligible validators for shard %d", shardID)
	}

	thresholdReputation := vm.CalculateAdaptiveThreshold(eligibleValidators)
	totalEligibleReputation := int64(0)
	for _, v := range eligibleValidators {
		totalEligibleReputation += v.Reputation.Load()
	}

	log.Printf("[Consensus] Shard %d: Starting dBFT for Block H:%d Hash:%x... Need > %.2f reputation (out of %d total from %d validators).",
		shardID, block.Header.Height, safeSlice(block.Hash, 4), float64(thresholdReputation), totalEligibleReputation, len(eligibleValidators))

	vm.mu.RUnlock()

	votesReputation := int64(0)
	agreeingSignatures := make(map[NodeID]Signature)
	var voteMu sync.Mutex

	// Use the public PrepareData method for consistent hash calculation
	powForHash := NewProofOfWork(block)
	dataToSign := powForHash.PrepareData(block.Header.Nonce)
	hashToSign := sha256.Sum256(dataToSign)

	// Verify the calculated hash against the stored block hash
	if !bytes.Equal(block.Hash, hashToSign[:]) {
		log.Printf("[Consensus] Block hash mismatch! Stored: %x, Calculated: %x", block.Hash, hashToSign[:])
		return nil, fmt.Errorf("block hash mismatch")
	}

	var wg sync.WaitGroup
	for _, validator := range eligibleValidators {
		wg.Add(1)
		go func(v *Validator) {
			defer wg.Done()
			// Validate PoW (optional, as hash match implies PoW was likely done, but good for defense)
			powInstance := NewProofOfWork(block)
			if !powInstance.Validate() {
				log.Printf("[Consensus] Shard %d: Validator %s found block invalid (PoW). Voting NO.", shardID, v.Node.ID)
				return // Don't sign if PoW is invalid
			}

			// Simulate voting probability (e.g., 98% vote yes)
			if vm.randSource.Float32() < 0.98 {
				// Sign the *correctly calculated* hashToSign
				signature, err := v.Node.SignData(hashToSign[:])
				if err != nil {
					log.Printf("[Consensus] Shard %d: Validator %s FAILED to sign block %x: %v", shardID, v.Node.ID, safeSlice(block.Hash, 4), err)
					return
				}

				// Simulate network delay
				time.Sleep(time.Duration(10+vm.randSource.Intn(50)) * time.Millisecond)

				// Verify the signature produced by the validator
				isValidSig := ecdsa.VerifyASN1(v.Node.PublicKey, hashToSign[:], signature)
				if isValidSig {
					voteMu.Lock()
					agreeingSignatures[v.Node.ID] = signature
					votesReputation += v.Reputation.Load()
					voteMu.Unlock()
				} else {
					log.Printf("[Consensus] Shard %d: Validator %s produced INVALID signature for block %x!", shardID, v.Node.ID, safeSlice(block.Hash, 4))
					// Penalize for invalid signature
					voteMu.Lock()
					vm.UpdateReputation(v.Node.ID, -5)
					v.Node.UpdateTrustScore(-0.1)
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

	// Update reputations asynchronously based on consensus outcome
	go func(validators []*Validator, reached bool, agreeing map[NodeID]Signature) {
		vm.mu.Lock()
		defer vm.mu.Unlock()

		for _, v := range validators {
			_, agreed := agreeing[v.Node.ID]
			currentRep := vm.ReputationScores[v.Node.ID]
			change := int64(0)

			if reached {
				if agreed {
					change = DbftConsensusReward // Use constant
				} else {
					change = DbftConsensusPenalty // Use constant
				}
			} else {
				// Penalize slightly even if consensus failed, if the validator didn't agree (or didn't vote)
				if !agreed {
					change = -1
				}
			}

			newRep := currentRep + change
			if newRep < 1 {
				newRep = 1 // Floor reputation at 1
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

		// Set finalizer IDs on the original block header
		finalizerIDs := make([]NodeID, 0, len(agreeingSignatures))
		for id := range agreeingSignatures {
			finalizerIDs = append(finalizerIDs, id)
		}
		sort.Slice(finalizerIDs, func(i, j int) bool { return finalizerIDs[i] < finalizerIDs[j] })
		block.Header.FinalitySignatures = finalizerIDs // Update the block directly

		return signedBlock, nil
	}

	// Consensus failed
	log.Printf("[Consensus] Shard %d: dBFT Consensus FAILED for Block H:%d Hash:%x... (%d/%d Reputation)",
		shardID, block.Header.Height, safeSlice(block.Hash, 4), votesReputation, thresholdReputation)
	return signedBlock, fmt.Errorf("dBFT consensus failed for block %x on shard %d (%d/%d reputation)", block.Hash, shardID, votesReputation, thresholdReputation)
}

// CalculateAdaptiveThreshold calculates the consensus threshold based on total reputation.
// Returns the minimum reputation required to reach consensus (> 2/3 of total eligible reputation).
func (vm *ValidatorManager) CalculateAdaptiveThreshold(eligibleValidators []*Validator) int64 { // Renamed from calculateAdaptiveThreshold
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

	// Calculate threshold: floor(totalReputation * 2 / 3)
	// Use big.Int for potentially large reputation sums to avoid overflow if necessary,
	// but int64 is likely sufficient for now.
	threshold := (totalReputation * 2) / 3

	return threshold
}

// SelectDelegateWithVRF selects a delegate using a Verifiable Random Function (VRF) for consensus.
// It iterates through eligible validators, evaluates their VRF, verifies the proof, and selects the one with the lowest valid output.
func (vm *ValidatorManager) SelectDelegateWithVRF(seed []byte, shardID uint64, blockHeight uint64) (*Validator, *VRFOutput, error) { // Return proof as well
	vm.mu.RLock() // Use RLock as we are primarily reading validator data
	eligible := vm.GetActiveValidatorsForShard(shardID)
	vm.mu.RUnlock() // Release lock after getting the list

	if len(eligible) == 0 {
		return nil, nil, fmt.Errorf("no eligible validators for VRF selection on shard %d", shardID)
	}

	var bestValidator *Validator
	var lowestValue *big.Int
	var bestProof *VRFOutput // Store the proof of the best validator

	// log.Printf("[Consensus] Starting VRF Delegate Selection for Shard %d (H:%d) with %d candidates. Seed: %x", shardID, blockHeight, len(eligible), seed)

	// Construct the base input for VRF evaluation (consistent across validators)
	baseInput := bytes.Join(
		[][]byte{
			seed,
			[]byte(fmt.Sprintf("%d", shardID)),
			[]byte(fmt.Sprintf("%d", blockHeight)),
		},
		[]byte{},
	)

	var selectionWg sync.WaitGroup
	resultChan := make(chan struct {
		validator *Validator
		proof     *VRFOutput
		value     *big.Int
		err       error
	}, len(eligible)) // Buffered channel

	// Evaluate VRF for each eligible validator concurrently
	for _, v := range eligible {
		selectionWg.Add(1)
		go func(validator *Validator) {
			defer selectionWg.Done()

			if validator.Node == nil || validator.Node.VRF == nil {
				// log.Printf("[Consensus] Validator %s has missing Node or VRF instance.", validator.Node.ID)
				resultChan <- struct {
					validator *Validator
					proof     *VRFOutput
					value     *big.Int
					err       error
				}{validator, nil, nil, fmt.Errorf("missing node or vrf")}
				return
			}

			// Append validator ID to base input for unique evaluation
			vrfInput := append(baseInput, []byte(validator.Node.ID)...)
			// log.Printf("[Consensus] Evaluating VRF for Validator %s with input: %x", validator.Node.ID, vrfInput)

			// Evaluate VRF using the validator's specific VRF instance
			output, err := validator.Node.VRF.Evaluate(vrfInput)
			if err != nil {
				// log.Printf("[Consensus] VRF evaluation failed for Validator %s: %v", validator.Node.ID, err)
				resultChan <- struct {
					validator *Validator
					proof     *VRFOutput
					value     *big.Int
					err       error
				}{validator, nil, nil, fmt.Errorf("evaluation failed: %w", err)}
				return
			}

			// Verify the proof using the validator's public key right away
			if !VerifyWithPublicKey(validator.Node.PublicKey, vrfInput, output) {
				// log.Printf("[Consensus] VRF proof verification failed for Validator %s", validator.Node.ID)
				// Optionally penalize the validator here (needs mutex if modifying reputation)
				// go vm.UpdateReputation(validator.Node.ID, -1) // Example concurrent penalty
				resultChan <- struct {
					validator *Validator
					proof     *VRFOutput
					value     *big.Int
					err       error
				}{validator, output, nil, fmt.Errorf("verification failed")}
				return
			}

			// If evaluation and verification succeeded, send result
			value := new(big.Int).SetBytes(output.Output)
			// log.Printf("[Consensus] Validator %s VRF output value: %s", validator.Node.ID, value.String())
			resultChan <- struct {
				validator *Validator
				proof     *VRFOutput
				value     *big.Int
				err       error
			}{validator, output, value, nil}

		}(v)
	}

	// Wait for all evaluations to complete and close channel
	go func() {
		selectionWg.Wait()
		close(resultChan)
	}()

	// Process results from the channel
	validResults := 0
	for result := range resultChan {
		if result.err == nil && result.value != nil {
			validResults++
			if lowestValue == nil || result.value.Cmp(lowestValue) < 0 {
				lowestValue = result.value
				bestValidator = result.validator
				bestProof = result.proof // Store the corresponding proof
			}
		} else {
			// Log the error for the specific validator if needed
			// log.Printf("[Consensus] VRF issue for validator %s: %v", result.validator.Node.ID, result.err)
		}
	}

	if bestValidator == nil {
		log.Printf("[Consensus] No valid VRF delegate found after evaluating %d candidates for shard %d (H:%d). Valid results: %d.", len(eligible), shardID, blockHeight, validResults)
		return nil, nil, fmt.Errorf("no valid delegate found for shard %d (H:%d)", shardID, blockHeight)
	}

	log.Printf("[Consensus] VRF selected Validator %s for Shard %d (H:%d)", bestValidator.Node.ID, shardID, blockHeight)
	// Return the best validator AND their corresponding proof
	return bestValidator, bestProof, nil
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

// SlashValidator penalizes a validator for adversarial behavior.
func (vm *ValidatorManager) SlashValidator(nodeID NodeID, reason string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	validator, exists := vm.Validators[nodeID]
	if !exists {
		log.Printf("[Slashing] Validator %s not found.", nodeID)
		return
	}

	// Decrease reputation and log the reason
	validator.Reputation.Add(-20) // Apply a significant penalty
	log.Printf("[Slashing] Validator %s slashed for reason: %s. New reputation: %d", nodeID, reason, validator.Reputation.Load())

	// Mark the validator as inactive if reputation falls below a critical threshold
	if validator.Reputation.Load() < 10 {
		validator.IsActive.Store(false)
		log.Printf("[Slashing] Validator %s marked as inactive due to low reputation.", nodeID)
	}

	// Adjust trust score
	validator.Node.UpdateTrustScore(-0.5)
}

// MonitorAdversarialBehavior monitors validators for adversarial behavior and applies slashing if necessary.
func (vm *ValidatorManager) MonitorAdversarialBehavior() {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	for _, validator := range vm.Validators {
		// Example: Check for repeated invalid signatures or failure to participate
		if validator.Reputation.Load() < 15 && validator.IsActive.Load() {
			log.Printf("[Adversarial] Validator %s flagged for potential adversarial behavior.", validator.Node.ID)
			vm.SlashValidator(validator.Node.ID, "Repeated invalid actions or failure to participate")
		}
	}
}

// Add advanced adversarial defense logic to ValidatorManager
func (vm *ValidatorManager) MonitorAdvancedAdversarialBehavior() {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	for _, validator := range vm.Validators {
		if validator.Reputation.Load() < 10 && validator.IsActive.Load() {
			log.Printf("[Adversarial] Validator %s flagged for adversarial behavior.", validator.Node.ID)
			vm.SlashValidator(validator.Node.ID, "Low reputation indicating adversarial behavior")
		}
		if validator.Node.TrustScore < 0.3 {
			log.Printf("[Adversarial] Validator %s has low trust score.", validator.Node.ID)
			vm.SlashValidator(validator.Node.ID, "Low trust score indicating adversarial behavior")
		}
	}
}

// VerifyAndSlash checks a validator's behavior during consensus and applies slashing if necessary.
func (vm *ValidatorManager) VerifyAndSlash(nodeID NodeID, isValid bool, reason string) {
	if !isValid {
		log.Printf("[Adversarial] Validator %s produced invalid behavior: %s", nodeID, reason)
		vm.SlashValidator(nodeID, reason)
	}
}

// Refining VerifyResponse to ensure error messages match test expectations
func (vm *ValidatorManager) VerifyResponse(nodeID NodeID, responseData Signature) error {
	vm.ChallengeMu.Lock()
	pending, exists := vm.PendingChallenges[nodeID]
	// Don't delete challenge yet, might need it for hash calculation
	vm.ChallengeMu.Unlock() // Unlock earlier

	if !exists {
		return fmt.Errorf("no pending challenge found for validator %s to verify response", nodeID)
	}

	// Check expiry FIRST
	if time.Now().After(pending.Expiry) {
		log.Printf("[Auth] Challenge response from %s expired.", nodeID)
		// Remove the expired challenge
		vm.ChallengeMu.Lock()
		delete(vm.PendingChallenges, nodeID)
		vm.ChallengeMu.Unlock()
		// Penalize
		vm.UpdateReputation(nodeID, -5)
		// Safely access validator node for trust score update
		vm.mu.RLock()
		v, vExists := vm.Validators[nodeID]
		vm.mu.RUnlock()
		if vExists {
			v.Node.UpdateTrustScore(-0.15)
		}
		return fmt.Errorf("challenge expired for validator %s", nodeID) // Return error here
	}

	// If not expired, proceed with verification
	vm.mu.RLock()
	v, vExists := vm.Validators[nodeID]
	vm.mu.RUnlock()
	if !vExists {
		// This case should ideally not happen if challenge existed, but handle defensively
		return fmt.Errorf("validator %s disappeared during challenge verification", nodeID)
	}

	// Ensure consistent nonce byte representation using binary.BigEndian
	hasher := sha256.New()
	hasher.Write(pending.ChallengeData)
	nonceBytes := make([]byte, 8) // Use fixed 8 bytes for uint64
	binary.BigEndian.PutUint64(nonceBytes, pending.ExpectedNonce)
	hasher.Write(nonceBytes)
	expectedHash := hasher.Sum(nil)

	// Remove the challenge *after* using its data but *before* potentially long crypto op
	vm.ChallengeMu.Lock()
	delete(vm.PendingChallenges, nodeID)
	vm.ChallengeMu.Unlock()

	if !v.Node.VerifySignature(expectedHash, responseData) {
		log.Printf("[Auth] Challenge response verification FAILED for %s. Expected Nonce: %d", nodeID, pending.ExpectedNonce)
		vm.UpdateReputation(nodeID, -10)
		v.Node.UpdateTrustScore(-0.25)
		return fmt.Errorf("invalid signature in challenge response for validator %s", nodeID)
	}

	// Successful verification
	log.Printf("[Auth] Challenge response verification SUCCESSFUL for %s.", nodeID)
	vm.UpdateReputation(nodeID, 2)
	v.Node.LastAttested = time.Now()
	v.Node.UpdateTrustScore(0.1)
	if !v.IsActive.Load() {
		v.IsActive.Store(true)
		log.Printf("[Auth] Reactivated validator %s after successful challenge response.", nodeID)
	}

	// Mark the node as authenticated upon successful verification
	v.Node.Authenticate()

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
