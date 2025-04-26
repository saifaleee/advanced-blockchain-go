package core_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/saifaleee/advanced-blockchain-go/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock VRF Tests ---

func TestMockVRF_Determinism(t *testing.T) {
	input := []byte("test-input")
	nodeID1 := core.NodeID("node1")
	nodeID2 := core.NodeID("node2")

	output1a := core.MockVRF(input, nodeID1)
	output1b := core.MockVRF(input, nodeID1)
	output2 := core.MockVRF(input, nodeID2)
	output3 := core.MockVRF([]byte("other-input"), nodeID1)

	require.NotNil(t, output1a)
	require.NotNil(t, output1b)
	require.NotNil(t, output2)
	require.NotNil(t, output3)

	// Same input, same node -> same output
	assert.Equal(t, output1a.Hash, output1b.Hash, "Same input/node should yield same hash")
	assert.Equal(t, output1a.Proof, output1b.Proof, "Same input/node should yield same proof")

	// Same input, different node -> different output
	assert.NotEqual(t, output1a.Hash, output2.Hash, "Different node should yield different hash")

	// Different input, same node -> different output
	assert.NotEqual(t, output1a.Hash, output3.Hash, "Different input should yield different hash")

	// Check basic proof structure (contains input and hash)
	assert.True(t, bytes.Contains(output1a.Proof, output1a.Input))
	assert.True(t, bytes.Contains(output1a.Proof, output1a.Hash))
}

func TestVerifyMockVRF(t *testing.T) {
	input := []byte("verify-input")
	nodeID := core.NodeID("verifierNode")

	// Correct output
	correctOutput := core.MockVRF(input, nodeID)
	assert.True(t, core.VerifyMockVRF(correctOutput, nodeID), "Verification should pass for correct output")

	// Incorrect NodeID for verification
	assert.False(t, core.VerifyMockVRF(correctOutput, core.NodeID("wrongNode")), "Verification should fail for wrong node ID")

	// Tampered Hash
	tamperedOutput := core.MockVRF(input, nodeID)
	tamperedOutput.Hash[0] ^= 0xff // Flip first byte
	assert.False(t, core.VerifyMockVRF(tamperedOutput, nodeID), "Verification should fail for tampered hash")

	// Nil output
	assert.False(t, core.VerifyMockVRF(nil, nodeID), "Verification should fail for nil output")
}

// --- Conflict Resolution Tests ---

func createTestConflictingItem(t *testing.T, id string, value string, nodeID core.NodeID, vrfInput []byte) *core.ConflictingItem {
	version := &core.StateVersion{
		Value:     []byte(value),
		Clock:     core.NewVectorClock(), // Keep clock simple for this test
		Source:    nodeID,
		Timestamp: time.Now(),
	}
	vrfOutput := core.MockVRF(vrfInput, nodeID)
	require.NotNil(t, vrfOutput)

	return &core.ConflictingItem{
		ID:        id,
		Version:   version,
		VRFInput:  vrfInput,
		VRFOutput: vrfOutput,
	}
}

func TestResolveConflictVRF_SingleItem(t *testing.T) {
	vrfInput := []byte("resolve-input")
	item1 := createTestConflictingItem(t, "item1", "valueA", "nodeA", vrfInput)

	winner, err := core.ResolveConflictVRF([]*core.ConflictingItem{item1})
	require.NoError(t, err)
	assert.Same(t, item1, winner)
}

func TestResolveConflictVRF_MultipleItems(t *testing.T) {
	vrfInput := []byte("resolve-input-multi")
	nodeA := core.NodeID("nodeA")
	nodeB := core.NodeID("nodeB")
	nodeC := core.NodeID("nodeC")

	itemA := createTestConflictingItem(t, "item", "valueA", nodeA, vrfInput)
	itemB := createTestConflictingItem(t, "item", "valueB", nodeB, vrfInput)
	itemC := createTestConflictingItem(t, "item", "valueC", nodeC, vrfInput)

	items := []*core.ConflictingItem{itemA, itemB, itemC}

	// Determine expected winner manually by comparing hashes
	expectedWinner := itemA
	if bytes.Compare(itemB.VRFOutput.Hash, expectedWinner.VRFOutput.Hash) < 0 {
		expectedWinner = itemB
	}
	if bytes.Compare(itemC.VRFOutput.Hash, expectedWinner.VRFOutput.Hash) < 0 {
		expectedWinner = itemC
	}

	winner, err := core.ResolveConflictVRF(items)
	require.NoError(t, err)
	assert.Same(t, expectedWinner, winner, fmt.Sprintf("Expected winner %s, got %s", expectedWinner.Version.Source, winner.Version.Source))
	t.Logf("VRF Hashes: A:%x, B:%x, C:%x", itemA.VRFOutput.Hash, itemB.VRFOutput.Hash, itemC.VRFOutput.Hash)
	t.Logf("Winner: %s", winner.Version.Source)
}

func TestResolveConflictVRF_EmptyInput(t *testing.T) {
	_, err := core.ResolveConflictVRF([]*core.ConflictingItem{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no items provided")
}

func TestResolveConflictVRF_MissingData(t *testing.T) {
	vrfInput := []byte("resolve-input-missing")
	item1 := createTestConflictingItem(t, "item1", "valueA", "nodeA", vrfInput)
	item2 := createTestConflictingItem(t, "item2", "valueB", "nodeB", vrfInput)
	item2.VRFOutput = nil // Simulate missing VRF output

	_, err := core.ResolveConflictVRF([]*core.ConflictingItem{item1, item2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing version or VRF output")

	item2.VRFOutput = core.MockVRF(vrfInput, "nodeB") // Restore VRF
	item2.Version = nil                               // Simulate missing version
	_, err = core.ResolveConflictVRF([]*core.ConflictingItem{item1, item2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing version or VRF output")
}

func TestResolveConflictVRF_InputMismatch(t *testing.T) {
	vrfInput1 := []byte("resolve-input-1")
	vrfInput2 := []byte("resolve-input-2")
	item1 := createTestConflictingItem(t, "item1", "valueA", "nodeA", vrfInput1)
	item2 := createTestConflictingItem(t, "item2", "valueB", "nodeB", vrfInput2)
	// Manually create mismatch
	item1.VRFOutput.Input = vrfInput2

	_, err := core.ResolveConflictVRF([]*core.ConflictingItem{item1, item2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VRF input mismatch")
}

// --- Entropy Detection Tests (Placeholder) ---

func TestDetectConflictEntropy_NoConflict(t *testing.T) {
	key := []byte("entropy-key")
	v1 := &core.StateVersion{Value: []byte("value"), Source: "nodeA"}
	v2 := &core.StateVersion{Value: []byte("value"), Source: "nodeB"}

	// Single version
	conflict, entropy := core.DetectConflictEntropy(key, []*core.StateVersion{v1})
	assert.False(t, conflict)
	assert.Equal(t, 0.0, entropy)

	// Multiple identical versions
	conflict, entropy = core.DetectConflictEntropy(key, []*core.StateVersion{v1, v2})
	assert.False(t, conflict)
	assert.Equal(t, 0.0, entropy)
}

func TestDetectConflictEntropy_Conflict(t *testing.T) {
	key := []byte("entropy-key-conflict")
	vA := &core.StateVersion{Value: []byte("valueA"), Source: "nodeA"}
	vB := &core.StateVersion{Value: []byte("valueB"), Source: "nodeB"}
	vC := &core.StateVersion{Value: []byte("valueA"), Source: "nodeC"} // Another A

	// Two different values (A, B) - Entropy = -(0.5*log2(0.5) + 0.5*log2(0.5)) = 1.0
	conflict, entropy := core.DetectConflictEntropy(key, []*core.StateVersion{vA, vB})
	assert.True(t, conflict, "Entropy should be > threshold (1.0 > 0.5)")
	assert.InDelta(t, 1.0, entropy, 0.001)

	// Three versions, two types (A, B, A) - Entropy = -( (2/3)*log2(2/3) + (1/3)*log2(1/3) ) ~ 0.918
	conflict, entropy = core.DetectConflictEntropy(key, []*core.StateVersion{vA, vB, vC})
	assert.True(t, conflict, "Entropy should be > threshold (~0.918 > 0.5)")
	assert.InDelta(t, 0.918, entropy, 0.001)
}

func TestDetectConflictEntropy_Empty(t *testing.T) {
	key := []byte("entropy-key-empty")
	conflict, entropy := core.DetectConflictEntropy(key, []*core.StateVersion{})
	assert.False(t, conflict)
	assert.Equal(t, 0.0, entropy)
}
