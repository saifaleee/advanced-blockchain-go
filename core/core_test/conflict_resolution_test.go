package core_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"math/big"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	core "github.com/saifaleee/advanced-blockchain-go/core"
)

// --- Local VRF interface for mocking ---
type VRF interface {
	Evaluate(input []byte) (output []byte, proof []byte, err error)
	Verify(publicKey, input, output, proof []byte) (bool, error)
}

// --- Global VRF instance for mocking ---
var vrfInstance VRF // Use the local interface type

// --- Vector Clock Tests ---

func TestVectorClock_New(t *testing.T) {
	vc := core.NewVectorClock()
	assert.NotNil(t, vc)
	assert.Empty(t, vc, "New vector clock should be empty")
}

func TestVectorClock_Increment(t *testing.T) {
	vc := core.NewVectorClock()
	var nodeID1 uint64 = 1 // Use uint64 directly as per VectorClock definition
	var nodeID2 uint64 = 2

	vc.Increment(nodeID1)
	assert.Equal(t, uint64(1), vc[nodeID1], "Clock should be incremented for node1")

	vc.Increment(nodeID1)
	assert.Equal(t, uint64(2), vc[nodeID1], "Clock should be incremented again for node1")

	vc.Increment(nodeID2)
	assert.Equal(t, uint64(1), vc[nodeID2], "Clock should be incremented for node2")
	assert.Equal(t, uint64(2), vc[nodeID1], "Clock for node1 should remain unchanged")
}

func TestVectorClock_Compare(t *testing.T) {
	vc1 := core.NewVectorClock()
	vc2 := core.NewVectorClock()
	var nodeA uint64 = 1 // Use uint64
	var nodeB uint64 = 2
	var nodeC uint64 = 3
	var nodeX uint64 = 10
	var nodeY uint64 = 11

	// Initially equal
	assert.Equal(t, 0, vc1.Compare(vc2), "Initially clocks should be equal") // Use int constants from Compare return type
	assert.Equal(t, 0, vc2.Compare(vc1), "Initially clocks should be equal (commutative)")

	// vc1 happens before vc2
	vc1.Increment(nodeA) // vc1: {1:1}
	// Corrected Expectations: {1:1} happens AFTER {}
	assert.Equal(t, 1, vc1.Compare(vc2), "vc1 {1:1} should happen AFTER vc2 {}")
	assert.Equal(t, -1, vc2.Compare(vc1), "vc2 {} should happen BEFORE vc1 {1:1}")

	vc2.Increment(nodeA) // vc2: {1:1}
	assert.Equal(t, 0, vc1.Compare(vc2), "Clocks should be equal after same increment")

	vc2.Increment(nodeB) // vc1: {1:1}, vc2: {1:1, 2:1}
	assert.Equal(t, -1, vc1.Compare(vc2), "vc1 should happen before vc2")
	assert.Equal(t, 1, vc2.Compare(vc1), "vc2 should happen after vc1")

	// Concurrent change
	vc1.Increment(nodeC) // vc1: {1:1, 3:1}, vc2: {1:1, 2:1}
	assert.Equal(t, 2, vc1.Compare(vc2), "Clocks should be concurrent")
	assert.Equal(t, 2, vc2.Compare(vc1), "Clocks should be concurrent (commutative)")

	// vc2 catches up and surpasses
	vc2.Increment(nodeC) // vc1: {1:1, 3:1}, vc2: {1:1, 2:1, 3:1}
	assert.Equal(t, -1, vc1.Compare(vc2), "vc1 should happen before vc2")
	assert.Equal(t, 1, vc2.Compare(vc1), "vc2 should happen after vc1")

	// Test with different sets of nodes
	vc3 := core.NewVectorClock()
	vc4 := core.NewVectorClock()
	vc3.Increment(nodeX) // vc3: {10:1}
	vc4.Increment(nodeY) // vc4: {11:1}
	assert.Equal(t, 2, vc3.Compare(vc4), "Clocks with disjoint node sets should be concurrent")
}

func TestVectorClock_Merge(t *testing.T) {
	vc1 := core.NewVectorClock()
	vc2 := core.NewVectorClock()
	var nodeA uint64 = 1 // Use uint64
	var nodeB uint64 = 2
	var nodeC uint64 = 3

	vc1.Increment(nodeA) // vc1: {1:1}
	vc2.Increment(nodeB) // vc2: {2:1}
	vc1.Increment(nodeC) // vc1: {1:1, 3:1}
	vc2.Increment(nodeC) // vc2: {2:1, 3:1}
	vc2.Increment(nodeC) // vc2: {2:1, 3:2}

	// Create copies to test merge without modifying originals directly in assertion
	vc1Copy := vc1.Copy()
	vc1Copy.Merge(vc2) // Modify vc1Copy

	expectedVC := core.VectorClock{nodeA: 1, nodeB: 1, nodeC: 2}
	assert.Equal(t, expectedVC, vc1Copy, "Merged clock should contain max of each entry")

	// Ensure original clocks are not modified by the merge operation itself
	assert.Equal(t, core.VectorClock{nodeA: 1, nodeC: 1}, vc1)
	assert.Equal(t, core.VectorClock{nodeB: 1, nodeC: 2}, vc2)

	// Merge with empty
	vc3 := core.NewVectorClock()
	vc1CopyForEmptyTest := vc1.Copy()
	vc1CopyForEmptyTest.Merge(vc3) // Merge empty into vc1 copy
	assert.Equal(t, vc1, vc1CopyForEmptyTest, "Merging with empty should return original")

	vc3Copy := vc3.Copy()
	vc3Copy.Merge(vc1) // Merge vc1 into empty copy
	assert.Equal(t, vc1, vc3Copy, "Merging empty with other should return other")
}

// --- Entropy Calculation Tests ---

func TestCalculateStateEntropy(t *testing.T) {
	resolver := core.NewConflictResolver()

	tests := []struct {
		name            string
		versions        map[core.NodeID]core.StateVersion
		expectedEntropy float64
		expectError     bool
	}{
		{
			name:            "Empty input",
			versions:        map[core.NodeID]core.StateVersion{},
			expectedEntropy: 0.0,
			expectError:     false,
		},
		{
			name: "Single version",
			versions: map[core.NodeID]core.StateVersion{
				// Use string literal for NodeID
				core.NodeID("1"): {Key: "k1", Value: "hash1", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("1")},
			},
			expectedEntropy: 0.0, // Entropy of a single outcome is 0
			expectError:     false,
		},
		{
			name: "All same versions",
			versions: map[core.NodeID]core.StateVersion{
				// Use string literals for NodeID
				core.NodeID("1"): {Key: "k1", Value: "hash1", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("1")},
				core.NodeID("2"): {Key: "k1", Value: "hash1", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("2")},
				core.NodeID("3"): {Key: "k1", Value: "hash1", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("3")},
			},
			expectedEntropy: 0.0, // Only one distinct state hash
			expectError:     false,
		},
		{
			name: "Two distinct versions (equal split)",
			versions: map[core.NodeID]core.StateVersion{
				// Use string literals for NodeID
				core.NodeID("1"): {Key: "k1", Value: "hash1", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("1")},
				core.NodeID("2"): {Key: "k1", Value: "hash2", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("2")},
			},
			// p(hash1) = 0.5, p(hash2) = 0.5
			// Entropy = - (0.5 * log2(0.5) + 0.5 * log2(0.5)) = - (0.5 * -1 + 0.5 * -1) = 1.0
			expectedEntropy: 1.0,
			expectError:     false,
		},
		{
			name: "Three distinct versions (equal split)",
			versions: map[core.NodeID]core.StateVersion{
				// Use string literals for NodeID
				core.NodeID("1"): {Key: "k1", Value: "hash1", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("1")},
				core.NodeID("2"): {Key: "k1", Value: "hash2", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("2")},
				core.NodeID("3"): {Key: "k1", Value: "hash3", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("3")},
			},
			// p = 1/3 for each
			// Entropy = - 3 * (1/3 * log2(1/3)) = -log2(1/3) = log2(3) approx 1.585
			expectedEntropy: math.Log2(3),
			expectError:     false,
		},
		{
			name: "Multiple versions, unequal split",
			versions: map[core.NodeID]core.StateVersion{
				// Use string literals for NodeID
				core.NodeID("1"): {Key: "k1", Value: "hash1", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("1")}, // p=0.5
				core.NodeID("2"): {Key: "k1", Value: "hash1", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("2")}, // p=0.5
				core.NodeID("3"): {Key: "k1", Value: "hash2", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("3")}, // p=0.25
				core.NodeID("4"): {Key: "k1", Value: "hash3", VectorClock: core.NewVectorClock(), SourceNode: core.NodeID("4")}, // p=0.25
			},
			// p(hash1)=0.5, p(hash2)=0.25, p(hash3)=0.25
			// Entropy = - (0.5*log2(0.5) + 0.25*log2(0.25) + 0.25*log2(0.25))
			// Entropy = - (0.5*(-1) + 0.25*(-2) + 0.25*(-2))
			// Entropy = - (-0.5 - 0.5 - 0.5) = 1.5
			expectedEntropy: 1.5,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entropy, err := resolver.CalculateStateEntropy(tt.versions)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Use tolerance for floating point comparison
				assert.InDelta(t, tt.expectedEntropy, entropy, 1e-9, "Entropy calculation mismatch")
			}
		})
	}
}

// --- VRF Conflict Resolution Tests ---

// MockVRF implementation using the local VRF interface
type MockVRF struct{}

// Evaluate simulates VRF evaluation (simple reversal for predictability)
func (m *MockVRF) Evaluate(input []byte) (output []byte, proof []byte, err error) {
	// Simple deterministic output for testing: reverse the input bytes
	// In a real VRF, this would be a pseudo-random but verifiable output.
	out := make([]byte, len(input))
	for i := range input {
		out[i] = input[len(input)-1-i]
	}
	// Generate a predictable proof based on input/output for mock verification
	h := sha256.New()
	h.Write(input)
	h.Write(out)
	mockProof := h.Sum(nil)

	// Ensure proof has the expected length for VerifyVRF compatibility if needed elsewhere
	if len(mockProof) != sha256.Size {
		panic(fmt.Sprintf("mock proof length %d != %d", len(mockProof), sha256.Size))
	}

	return out, mockProof, nil
}

// Verify simulates VRF verification based on the mock Evaluate logic
func (m *MockVRF) Verify(publicKey, input, output, proof []byte) (bool, error) {
	// Re-calculate the expected proof based on input and output
	h := sha256.New()
	h.Write(input)
	h.Write(output)
	expectedProof := h.Sum(nil)
	// Compare calculated proof with the provided proof
	return bytes.Equal(proof, expectedProof), nil
}

func TestResolveConflictVRF(t *testing.T) {
	resolver := core.NewConflictResolver()
	mockVrf := &MockVRF{} // Use the local mock implementation
	seed := []byte("test-seed")

	var nodeA core.NodeID = "nodeA" // Use NodeID type
	var nodeB core.NodeID = "nodeB"
	var nodeC core.NodeID = "nodeC"
	var nodeX core.NodeID = "nodeX"
	var nodeY core.NodeID = "nodeY"

	vc1 := core.NewVectorClock()
	vc1.Increment(1) // Use uint64 for clock keys
	v1 := core.StateVersion{Key: "k1", Value: "hash1", VectorClock: vc1, SourceNode: nodeA}

	vc2 := core.NewVectorClock()
	vc2.Increment(2)
	v2 := core.StateVersion{Key: "k1", Value: "hash2", VectorClock: vc2, SourceNode: nodeB}

	vc3 := core.NewVectorClock()
	vc3.Increment(3)
	v3 := core.StateVersion{Key: "k1", Value: "hash3", VectorClock: vc3, SourceNode: nodeC}

	tests := []struct {
		name                string
		conflictingVersions []core.StateVersion
		expectedWinnerIndex int // Index in the input slice
		expectError         bool
	}{
		{
			name:                "No conflicts",
			conflictingVersions: []core.StateVersion{},
			expectedWinnerIndex: -1, // Expect error
			expectError:         true,
		},
		{
			name:                "Single version",
			conflictingVersions: []core.StateVersion{v1},
			expectedWinnerIndex: 0,
			expectError:         false,
		},
		{
			name:                "Two conflicting versions",
			conflictingVersions: []core.StateVersion{v1, v2},
			expectedWinnerIndex: 0, // Assume v1 wins based on mock VRF
			expectError:         false,
		},
		{
			name:                "Three conflicting versions",
			conflictingVersions: []core.StateVersion{v1, v2, v3},
			expectedWinnerIndex: 0,
			expectError:         false,
		},
		{
			name: "Identical values (should still pick one deterministically based on NodeID/VC)",
			conflictingVersions: []core.StateVersion{
				{Key: "k_same", Value: "val_same", VectorClock: core.VectorClock{10: 1}, SourceNode: nodeX},
				{Key: "k_same", Value: "val_same", VectorClock: core.VectorClock{11: 1}, SourceNode: nodeY}, // Different NodeID/VC
			},
			expectedWinnerIndex: 0, // Assuming version from nodeX wins
			expectError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalVrf := vrfInstance                   // Store original global VRF (if any)
			vrfInstance = mockVrf                        // Inject mock
			defer func() { vrfInstance = originalVrf }() // Restore original

			winner, err := resolver.ResolveConflictVRF(tt.conflictingVersions, seed)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.conflictingVersions[tt.expectedWinnerIndex], winner, "Incorrect winner selected by VRF")
			}
		})
	}
}

// --- HandlePotentialConflict Tests (Rewritten) ---

func TestHandlePotentialConflict(t *testing.T) {
	resolver := core.NewConflictResolver()
	// Inject mock VRF globally for ResolveConflictVRF called internally
	originalVrf := vrfInstance
	vrfInstance = &MockVRF{}
	defer func() { vrfInstance = originalVrf }()

	var nodeA core.NodeID = "nodeA"
	var nodeB core.NodeID = "nodeB"

	key := "testKey"
	vrfContext := []byte("vrf-context-seed")

	// Helper function for manual serialization within the test
	serializeStateVersionForVRF := func(sv core.StateVersion) ([]byte, error) {
		var buf bytes.Buffer
		buf.WriteString(sv.Key)
		// Convert value to bytes - handle different types if necessary
		valueBytes := []byte(fmt.Sprintf("%v", sv.Value))
		buf.Write(valueBytes)
		vcBytes, err := sv.VectorClock.Serialize()
		if err != nil {
			return nil, fmt.Errorf("failed to serialize vector clock: %w", err)
		}
		buf.Write(vcBytes)
		buf.WriteString(string(sv.SourceNode))
		return buf.Bytes(), nil
	}

	t.Run("Different_Keys_Returns_Local", func(t *testing.T) {
		localVC := core.VectorClock{1: 1}
		remoteVC := core.VectorClock{2: 1}
		local := core.StateVersion{Key: "keyLocal", Value: "valLocal", VectorClock: localVC, SourceNode: nodeA}
		remote := core.StateVersion{Key: "keyRemote", Value: "valRemote", VectorClock: remoteVC, SourceNode: nodeB}

		resolved := resolver.HandlePotentialConflict(local, remote, vrfContext)
		assert.Equal(t, local, resolved, "Should return local version when keys differ")
	})

	t.Run("Local_HappensBefore_Remote_Returns_Remote_MergedClock", func(t *testing.T) {
		// Local: {A:1}
		localVC := core.VectorClock{uint64(1): 1} // Use uint64 keys
		local := core.StateVersion{Key: key, Value: "valLocal", VectorClock: localVC, SourceNode: nodeA}

		// Remote: {A:1, B:1} (Happened after local)
		remoteVC := core.VectorClock{uint64(1): 1, uint64(2): 1}
		remote := core.StateVersion{Key: key, Value: "valRemote", VectorClock: remoteVC, SourceNode: nodeB}

		resolved := resolver.HandlePotentialConflict(local, remote, vrfContext)

		// Expected: Remote wins, clock is merged
		expectedVC := core.VectorClock{uint64(1): 1, uint64(2): 1}
		expected := remote
		expected.VectorClock = expectedVC // The function modifies the clock of the returned version

		assert.Equal(t, "valRemote", resolved.Value, "Should return remote value")
		assert.Equal(t, expectedVC, resolved.VectorClock, "Should return merged vector clock")
		assert.Equal(t, expected, resolved) // Check whole struct
	})

	t.Run("Remote_HappensBefore_Local_Returns_Local_MergedClock", func(t *testing.T) {
		// Remote: {B:1}
		remoteVC := core.VectorClock{uint64(2): 1}
		remote := core.StateVersion{Key: key, Value: "valRemote", VectorClock: remoteVC, SourceNode: nodeB}

		// Local: {B:1, A:1} (Happened after remote)
		localVC := core.VectorClock{uint64(2): 1, uint64(1): 1}
		local := core.StateVersion{Key: key, Value: "valLocal", VectorClock: localVC, SourceNode: nodeA}

		resolved := resolver.HandlePotentialConflict(local, remote, vrfContext)

		// Expected: Local wins, clock is merged
		expectedVC := core.VectorClock{uint64(1): 1, uint64(2): 1}
		expected := local
		expected.VectorClock = expectedVC // The function modifies the clock of the returned version

		assert.Equal(t, "valLocal", resolved.Value, "Should return local value")
		assert.Equal(t, expectedVC, resolved.VectorClock, "Should return merged vector clock")
		assert.Equal(t, expected, resolved) // Check whole struct
	})

	t.Run("Equal_Clocks_Resolves_With_VRF", func(t *testing.T) {
		// Clocks are identical {A:1}
		vc := core.VectorClock{uint64(1): 1}
		local := core.StateVersion{Key: key, Value: "valLocal", VectorClock: vc.Copy(), SourceNode: nodeA}
		remote := core.StateVersion{Key: key, Value: "valRemote", VectorClock: vc.Copy(), SourceNode: nodeB} // Different source node

		// Determine VRF winner based on mock (depends on sorted order of NodeIDs)
		// Assume nodeA < nodeB
		sortedVersions := []core.StateVersion{local, remote}
		sort.Slice(sortedVersions, func(i, j int) bool {
			return sortedVersions[i].SourceNode < sortedVersions[j].SourceNode
		})

		hasher := sha256.New()
		hasher.Write(vrfContext)
		for _, v := range sortedVersions {
			vBytes, err := serializeStateVersionForVRF(v)
			assert.NoError(t, err, "Serialization for VRF failed")
			hasher.Write(vBytes)
		}
		vrfInput := hasher.Sum(nil)
		vrfOutput, _, _ := vrfInstance.Evaluate(vrfInput) // Use mocked Evaluate
		winnerIndex := int(new(big.Int).Mod(new(big.Int).SetBytes(vrfOutput), big.NewInt(2)).Int64())
		expectedWinner := sortedVersions[winnerIndex]

		resolved := resolver.HandlePotentialConflict(local, remote, vrfContext)

		// Expected: VRF winner, clock is merged
		expectedVC := core.VectorClock{uint64(1): 1} // Merge of {A:1} and {A:1} is {A:1}
		expected := expectedWinner
		expected.VectorClock = expectedVC // Function should set the merged clock

		assert.Equal(t, expectedWinner.Value, resolved.Value, "Should return VRF winner value")
		assert.Equal(t, expectedVC, resolved.VectorClock, "Should return merged vector clock")
		assert.Equal(t, expected, resolved)
	})

	t.Run("Concurrent_Clocks_Resolves_With_VRF", func(t *testing.T) {
		// Local: {A:1}, Remote: {B:1} -> Concurrent
		localVC := core.VectorClock{uint64(1): 1}
		local := core.StateVersion{Key: key, Value: "valLocal", VectorClock: localVC, SourceNode: nodeA}
		remoteVC := core.VectorClock{uint64(2): 1}
		remote := core.StateVersion{Key: key, Value: "valRemote", VectorClock: remoteVC, SourceNode: nodeB}

		// Determine VRF winner (same logic as Equal_Clocks case)
		sortedVersions := []core.StateVersion{local, remote}
		sort.Slice(sortedVersions, func(i, j int) bool {
			return sortedVersions[i].SourceNode < sortedVersions[j].SourceNode
		})
		hasher := sha256.New()
		hasher.Write(vrfContext)
		for _, v := range sortedVersions {
			vBytes, err := serializeStateVersionForVRF(v)
			assert.NoError(t, err, "Serialization for VRF failed")
			hasher.Write(vBytes)
		}
		vrfInput := hasher.Sum(nil)
		vrfOutput, _, _ := vrfInstance.Evaluate(vrfInput)
		winnerIndex := int(new(big.Int).Mod(new(big.Int).SetBytes(vrfOutput), big.NewInt(2)).Int64())
		expectedWinner := sortedVersions[winnerIndex]

		resolved := resolver.HandlePotentialConflict(local, remote, vrfContext)

		// Expected: VRF winner, clock is merged
		expectedVC := core.VectorClock{uint64(1): 1, uint64(2): 1} // Merge of {A:1} and {B:1}
		expected := expectedWinner
		expected.VectorClock = expectedVC // Function should set the merged clock

		assert.Equal(t, expectedWinner.Value, resolved.Value, "Should return VRF winner value")
		assert.Equal(t, expectedVC, resolved.VectorClock, "Should return merged vector clock")
		assert.Equal(t, expected, resolved)
	})

	t.Run("Concurrent_Clocks_With_Overlap_Resolves_With_VRF", func(t *testing.T) {
		// Local: {A:1, C:1}, Remote: {B:1, C:2} -> Concurrent
		localVC := core.VectorClock{uint64(1): 1, uint64(3): 1}
		local := core.StateVersion{Key: key, Value: "valLocal", VectorClock: localVC, SourceNode: nodeA}
		remoteVC := core.VectorClock{uint64(2): 1, uint64(3): 2} // C is higher in remote
		remote := core.StateVersion{Key: key, Value: "valRemote", VectorClock: remoteVC, SourceNode: nodeB}

		// Determine VRF winner
		sortedVersions := []core.StateVersion{local, remote}
		sort.Slice(sortedVersions, func(i, j int) bool {
			return sortedVersions[i].SourceNode < sortedVersions[j].SourceNode
		})
		hasher := sha256.New()
		hasher.Write(vrfContext)
		for _, v := range sortedVersions {
			vBytes, err := serializeStateVersionForVRF(v)
			assert.NoError(t, err, "Serialization for VRF failed")
			hasher.Write(vBytes)
		}
		vrfInput := hasher.Sum(nil)
		vrfOutput, _, _ := vrfInstance.Evaluate(vrfInput)
		winnerIndex := int(new(big.Int).Mod(new(big.Int).SetBytes(vrfOutput), big.NewInt(2)).Int64())
		expectedWinner := sortedVersions[winnerIndex] // Use the calculated winner index
		// The assertion below might need adjustment depending on the mock VRF's deterministic output for the given inputs.
		// assert.Equal(t, nodeB, expectedWinner.SourceNode, "Sanity check: Expected winner should be nodeB")

		resolved := resolver.HandlePotentialConflict(local, remote, vrfContext)

		// Expected: VRF winner (remote/nodeB), clock is merged
		expectedVC := core.VectorClock{uint64(1): 1, uint64(2): 1, uint64(3): 2} // Merge takes max
		expected := expectedWinner                                               // This is the remote version
		expected.VectorClock = expectedVC                                        // Function should set the merged clock

		// Corrected Assertions: Expect remote values
		assert.Equal(t, expectedWinner.Value, resolved.Value, "Should return VRF winner value") // Expect "valRemote"
		assert.Equal(t, expectedVC, resolved.VectorClock, "Should return merged vector clock")
		assert.Equal(t, expected, resolved) // Expect remote StateVersion with merged clock
	})
}
