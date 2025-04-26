package core

import (
	"crypto/sha256"
	"fmt"
	"log"
	"math"
	"math/big"
	"sort"
	"sync"
)

// StateVersion represents a specific version of a piece of state, identified by its key and vector clock.
type StateVersion struct {
	Key         string      // Identifier for the piece of state (e.g., account address, contract ID)
	Value       interface{} // The actual state value (can be simplified for simulation)
	VectorClock VectorClock // Use map[uint64]uint64 type from types.go
	SourceNode  NodeID      // Node proposing this version (optional, for context)
}

// ConflictResolver manages conflict detection and resolution strategies.
type ConflictResolver struct {
	mu sync.RWMutex
	// Configuration or state for the resolver can be added here if needed
}

// NewConflictResolver creates a new conflict resolver.
func NewConflictResolver() *ConflictResolver {
	return &ConflictResolver{}
}

// --- Entropy-Based Conflict Detection ---

// CalculateStateEntropy measures the diversity of state versions for a given key.
// Higher entropy indicates more disagreement or divergence among replicas.
// Input: A map where keys are node IDs (or replica identifiers) and values are the StateVersion they hold for a specific key.
func (cr *ConflictResolver) CalculateStateEntropy(versions map[NodeID]StateVersion) (float64, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	if len(versions) == 0 {
		return 0.0, nil // No versions, no entropy
	}

	// Count occurrences of each unique state representation (e.g., based on Value or a hash of Value)
	valueCounts := make(map[string]int)
	totalVersions := 0
	for _, version := range versions {
		// For simplicity, use a string representation of the value.
		// In a real system, you might hash the value or use a more robust comparison.
		valueStr := fmt.Sprintf("%v", version.Value)
		valueCounts[valueStr]++
		totalVersions++
	}

	if totalVersions == 0 {
		return 0.0, nil
	}

	entropy := 0.0
	for _, count := range valueCounts {
		probability := float64(count) / float64(totalVersions)
		if probability > 0 {
			entropy -= probability * math.Log2(probability)
		}
	}

	// Normalize entropy? Max entropy is log2(N) where N is number of unique states.
	// For now, return the raw entropy.
	log.Printf("[Conflict] Calculated state entropy: %.4f for key '%s' across %d versions (%d unique values)",
		entropy, getFirstKey(versions), totalVersions, len(valueCounts))

	return entropy, nil
}

// Helper to get the key from the first version (assuming all versions are for the same key)
func getFirstKey(versions map[NodeID]StateVersion) string {
	for _, v := range versions {
		return v.Key // Return the key of the first element found
	}
	return "<unknown>"
}

// --- Vector Clock Based Conflict Detection ---

// DetectCausalConflict checks if two state versions have a causal conflict based on their vector clocks.
// Returns true if they conflict (concurrent). Uses the VectorClock type from types.go.
func (cr *ConflictResolver) DetectCausalConflict(v1, v2 StateVersion) bool {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	// Use the Compare method from the VectorClock defined in types.go (map[uint64]uint64)
	comparison := v1.VectorClock.Compare(v2.VectorClock) // Returns -1 (Before), 0 (Equal), 1 (After), 2 (Concurrent)

	// Conflict exists if the clocks are concurrent (2) according to types.go definition
	isConflict := comparison == 2 // Concurrent

	// Existing logic: if values differ even if ordered, it's still often treated as a conflict needing resolution downstream.
	// For now, explicit conflict detection relies only on concurrency.

	if isConflict {
		log.Printf("[Conflict] Detected causal conflict (Concurrency) for key '%s' between nodes %s (%v) and %s (%v)",
			v1.Key, v1.SourceNode, v1.VectorClock, v2.SourceNode, v2.VectorClock)
	}

	return isConflict
}

// --- Probabilistic Conflict Resolution (Simulated VRF) ---

// SimulatedVRFOutput represents the output of our simulated VRF.
type SimulatedVRFOutput struct {
	RandomValue *big.Int // A pseudo-random value derived from the input
	Proof       []byte   // A dummy proof
}

// SimulateVRFEvaluation simulates the evaluation of a VRF.
// In a real VRF, this would involve a private key. Here we use a simple hash.
// Input: A byte slice representing the data to evaluate the VRF on (e.g., concatenation of conflicting state hashes and context).
func (cr *ConflictResolver) SimulateVRFEvaluation(input []byte) SimulatedVRFOutput {
	// Use SHA256 as a pseudo-random function. This is NOT cryptographically secure like a real VRF.
	hash := sha256.Sum256(input)
	randomValue := new(big.Int).SetBytes(hash[:]) // Use the hash output as the random value

	// Create a dummy proof (e.g., the input itself or part of the hash)
	dummyProof := hash[16:]

	log.Printf("[Conflict] Simulated VRF evaluation on input hash %x -> RandomValue: %s", sha256.Sum256(input), randomValue.String())

	return SimulatedVRFOutput{
		RandomValue: randomValue,
		Proof:       dummyProof,
	}
}

// SimulateVRFVerification simulates the verification of a VRF output.
// In a real VRF, this uses a public key. Here, we just re-hash.
func (cr *ConflictResolver) SimulateVRFVerification(input []byte, output SimulatedVRFOutput) bool {
	// Re-simulate evaluation to check consistency. A real verification is different.
	expectedOutput := cr.SimulateVRFEvaluation(input)
	isValid := expectedOutput.RandomValue.Cmp(output.RandomValue) == 0 // Compare big.Int values
	// In a real scenario, proof verification would also happen.
	log.Printf("[Conflict] Simulated VRF verification for input hash %x: %v", sha256.Sum256(input), isValid)
	return isValid
}

// ResolveConflictVRF uses a simulated VRF output to probabilistically choose a winning state version.
// Input: A slice of conflicting StateVersion objects and the VRF input data (context).
// Returns the chosen winning StateVersion.
func (cr *ConflictResolver) ResolveConflictVRF(conflictingVersions []StateVersion, vrfInputContext []byte) (StateVersion, error) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if len(conflictingVersions) == 0 {
		return StateVersion{}, fmt.Errorf("no conflicting versions provided")
	}
	if len(conflictingVersions) == 1 {
		return conflictingVersions[0], nil // Only one version, it wins by default
	}

	// 1. Prepare VRF Input: Combine context with info about conflicting versions
	//    Sort versions to ensure deterministic input order.
	sort.Slice(conflictingVersions, func(i, j int) bool {
		// Sort primarily by node ID for determinism
		return conflictingVersions[i].SourceNode < conflictingVersions[j].SourceNode
	})

	hasher := sha256.New()
	hasher.Write(vrfInputContext) // Include external context (e.g., block hash, timestamp)
	for _, v := range conflictingVersions {
		// Include key, value representation, and vector clock in the hash
		hasher.Write([]byte(v.Key))
		hasher.Write([]byte(fmt.Sprintf("%v", v.Value)))
		clockBytes, err := v.VectorClock.Serialize() // Use Serialize method from types.go
		if err != nil {
			log.Printf("[Error] Failed to marshal vector clock for VRF input: %v", err)
			// Decide how to handle - skip this version's clock? return error?
			// Skipping for now, might slightly bias VRF but avoids failure.
			continue
		}
		hasher.Write(clockBytes)
		hasher.Write([]byte(v.SourceNode))
	}
	vrfInput := hasher.Sum(nil)

	// 2. Simulate VRF Evaluation
	vrfOutput := cr.SimulateVRFEvaluation(vrfInput)

	// Optional: Simulate Verification (mostly for logging here)
	cr.SimulateVRFVerification(vrfInput, vrfOutput)

	// 3. Choose Winner based on VRF output
	//    Use the VRF random value modulo the number of conflicting versions to pick an index.
	numVersions := big.NewInt(int64(len(conflictingVersions)))
	winnerIndexBig := new(big.Int).Mod(vrfOutput.RandomValue, numVersions)
	winnerIndex := int(winnerIndexBig.Int64()) // Safe conversion as numVersions is small

	if winnerIndex < 0 || winnerIndex >= len(conflictingVersions) {
		// Should not happen with modulo arithmetic, but safety check
		log.Printf("[Error] VRF winner index out of bounds: %d (NumVersions: %d). Defaulting to index 0.", winnerIndex, len(conflictingVersions))
		winnerIndex = 0
	}

	winner := conflictingVersions[winnerIndex]

	log.Printf("[Conflict] VRF-based resolution for key '%s': Chose version from Node %s (Index %d) based on VRF value %s (mod %d)",
		winner.Key, winner.SourceNode, winnerIndex, vrfOutput.RandomValue.String(), numVersions.Int64())

	return winner, nil
}

// --- Placeholder for Integration ---
// HandlePotentialConflict is a placeholder showing where detection and resolution might be called.
// This would likely be integrated into the state update or block processing logic.
// Input: localVersion is the current state, remoteVersion is the incoming potentially conflicting state.
// Returns the resolved state version that should be persisted.
func (cr *ConflictResolver) HandlePotentialConflict(localVersion, remoteVersion StateVersion, vrfContext []byte) StateVersion {
	if localVersion.Key != remoteVersion.Key {
		log.Printf("[Warn] HandlePotentialConflict called with different keys: '%s' and '%s'. Ignoring remote.", localVersion.Key, remoteVersion.Key)
		return localVersion // Should not happen if called correctly
	}

	// 1. Detect Conflict using Vector Clocks (based on types.go definition)
	isConflict := cr.DetectCausalConflict(localVersion, remoteVersion) // Checks for concurrency (returns true if comparison == 2)

	// Use the Compare method from the VectorClock defined in types.go (map[uint64]uint64)
	comparison := localVersion.VectorClock.Compare(remoteVersion.VectorClock) // Returns -1, 0, 1, 2

	// If not strictly concurrent based on clocks, decide how to proceed based on order.
	if !isConflict {
		if comparison == -1 { // local happened before remote (remote is later)
			log.Printf("[Conflict] No direct clock conflict for key '%s': Remote version (%v) happened after local (%v). Accepting remote.", remoteVersion.Key, remoteVersion.VectorClock, localVersion.VectorClock)
			// Merge clocks to preserve history from both paths
			mergedClock := remoteVersion.VectorClock.Copy()
			mergedClock.Merge(localVersion.VectorClock) // Ensure local events are included
			remoteVersion.VectorClock = mergedClock
			return remoteVersion
		} else if comparison == 1 { // remote happened before local (local is later)
			log.Printf("[Conflict] No direct clock conflict for key '%s': Local version (%v) happened after remote (%v). Keeping local.", localVersion.Key, localVersion.VectorClock, remoteVersion.VectorClock)
			// Merge clocks to preserve history from both paths
			mergedClock := localVersion.VectorClock.Copy()
			mergedClock.Merge(remoteVersion.VectorClock)
			localVersion.VectorClock = mergedClock
			return localVersion
		} else if comparison == 0 { // Equal clocks - unusual, implies same history. Treat as conflict for resolution.
			log.Printf("[Conflict] Equal vector clocks (%v) for key '%s'. Treating as conflict for VRF resolution.", localVersion.VectorClock, localVersion.Key)
			isConflict = true // Trigger VRF resolution
		}
		// Note: comparison == 2 (Concurrent) is already handled by isConflict being true initially.
	} else {
		log.Printf("[Conflict] Concurrent vector clocks (%v vs %v) detected for key '%s'. Using VRF.", localVersion.VectorClock, remoteVersion.VectorClock, localVersion.Key)
		// isConflict is already true
	}

	// If conflict was detected initially or determined above, resolve it using VRF.
	if isConflict {
		log.Printf("[Conflict] Resolving conflict for key '%s' between local (Node %s, VC %v) and remote (Node %s, VC %v) using VRF.",
			localVersion.Key, localVersion.SourceNode, localVersion.VectorClock, remoteVersion.SourceNode, remoteVersion.VectorClock)
		conflicting := []StateVersion{localVersion, remoteVersion}

		// Resolve Conflict using VRF
		resolvedVersion, err := cr.ResolveConflictVRF(conflicting, vrfContext)
		if err != nil {
			log.Printf("[Error] Failed to resolve conflict using VRF for key '%s': %v. Defaulting to local version.", localVersion.Key, err)
			// Return local version as a fallback strategy on error
			return localVersion
		}

		log.Printf("[Conflict] Conflict for key '%s' resolved via VRF. Winning Node: %s, Winning VC: %v, Winning Value: '%v'",
			resolvedVersion.Key, resolvedVersion.SourceNode, resolvedVersion.VectorClock, resolvedVersion.Value)

		// Important: The resolved version should likely have a merged or updated vector clock
		// reflecting the resolution event itself. For simplicity, we currently return the winner's
		// original clock. A more advanced implementation would merge clocks here.
		mergedClock := localVersion.VectorClock.Copy() // Start with local
		mergedClock.Merge(remoteVersion.VectorClock)   // Merge remote
		// Potentially increment clock for the node performing the resolution? Needs node context.
		resolvedVersion.VectorClock = mergedClock // Use merged clock for resolved state
		log.Printf("[Conflict] Setting resolved state VC for key '%s' to merged clock: %v", resolvedVersion.Key, mergedClock)

		return resolvedVersion
	}

	// Should only reach here if comparison was -1 or 1 (no conflict)
	// and the function returned the appropriate version earlier.
	// Adding a fallback just in case, but indicates logic error if reached.
	log.Printf("[Warn] HandlePotentialConflict reached unexpected end state for key '%s'. Defaulting to local.", localVersion.Key)
	return localVersion
}
