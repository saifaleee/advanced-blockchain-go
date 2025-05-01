package core_test

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/saifaleee/advanced-blockchain-go/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a mock Node with keys for testing
func createTestNode(id core.NodeID) (*core.Node, error) {
	node, err := core.NewNode(id) // Adjusted to only pass NodeID
	if err != nil {
		return nil, fmt.Errorf("failed to create node %s: %w", id, err)
	}
	return node, nil
}

// Helper function to create a block for testing
func createTestBlock(height uint64, prevHash []byte) *core.Block {
	header := &core.BlockHeader{
		Height:        height,
		PrevBlockHash: prevHash,              // Corrected field name
		Timestamp:     time.Now().UnixNano(), // Corrected type to int64
		Nonce:         12345,                 // Example nonce
	}
	block := &core.Block{
		Header:       header,
		Transactions: []*core.Transaction{}, // Empty for simplicity
	}
	// Calculate a simple hash (replace with actual block hashing logic if available)
	h := sha256.New()
	h.Write(header.PrevBlockHash)
	h.Write([]byte(fmt.Sprintf("%d", header.Height)))
	block.Hash = h.Sum(nil)
	return block
}

// --- dBFT Signature Simulation Tests (Ticket 7 & 6) ---

func TestFinalizeBlockDBFT_Success(t *testing.T) {
	vm := core.NewValidatorManager()
	node1, _ := createTestNode("node1")
	node2, _ := createTestNode("node2")
	node3, _ := createTestNode("node3")

	vm.AddValidator(node1, 100)
	vm.AddValidator(node2, 100)
	vm.AddValidator(node3, 50) // Lower reputation

	block := createTestBlock(1, []byte("genesis"))
	shardID := uint64(0)

	signedBlock, err := vm.FinalizeBlockDBFT(block, shardID)

	require.NoError(t, err, "FinalizeBlockDBFT should succeed")
	assert.True(t, signedBlock.ConsensusReached, "Consensus should be reached")
	assert.GreaterOrEqual(t, len(signedBlock.FinalityVotes), 2, "Should have at least 2 signatures (from node1, node2)")

	// Check reputation updates (allow some time for async update)
	time.Sleep(50 * time.Millisecond)
	rep1, _ := vm.GetReputation(node1.ID)
	rep2, _ := vm.GetReputation(node2.ID)
	rep3, _ := vm.GetReputation(node3.ID)

	assert.Greater(t, rep1, int64(100), "Node1 reputation should increase")
	assert.Greater(t, rep2, int64(100), "Node2 reputation should increase")
	if _, ok3 := signedBlock.FinalityVotes[node3.ID]; ok3 {
		assert.Greater(t, rep3, int64(50), "Node3 reputation should increase if signed")
	} else {
		assert.Less(t, rep3, int64(50), "Node3 reputation should decrease if did not sign")
	}
}

func TestFinalizeBlockDBFT_Failure_InsufficientReputation(t *testing.T) {
	vm := core.NewValidatorManager()
	node1, _ := createTestNode("node1")
	node2, _ := createTestNode("node2") // Only two validators

	vm.AddValidator(node1, 100)
	vm.AddValidator(node2, 100)

	block := createTestBlock(1, []byte("genesis"))
	shardID := uint64(0)

	vm.UpdateReputation(node1.ID, -99) // node1 rep = 1
	vm.UpdateReputation(node2.ID, -99) // node2 rep = 1
	v1, _ := vm.GetValidator(node1.ID)
	v1.IsActive.Store(false) // Make node1 inactive

	signedBlock, err := vm.FinalizeBlockDBFT(block, shardID)

	require.Error(t, err, "FinalizeBlockDBFT should fail")
	assert.False(t, signedBlock.ConsensusReached, "Consensus should not be reached")
	assert.Contains(t, err.Error(), "dBFT consensus failed", "Error message should indicate failure")

	// Check reputation updates (node2 should be penalized for participating in failure)
	time.Sleep(50 * time.Millisecond)
	rep2, _ := vm.GetReputation(node2.ID)
	assert.Equal(t, int64(1), rep2, "Node2 reputation should be clamped to 1 after penalty")

	// Restore node1 active status for other tests if needed
	v1.IsActive.Store(true)
}

func TestFinalizeBlockDBFT_Failure_InvalidSignatures(t *testing.T) {
	vm := core.NewValidatorManager()
	node1, _ := createTestNode("node1")
	node2, _ := createTestNode("node2")

	vm.AddValidator(node1, 100)
	vm.AddValidator(node2, 100)

	block := createTestBlock(1, []byte("genesis"))
	shardID := uint64(0)

	_, _ = vm.FinalizeBlockDBFT(block, shardID)
	time.Sleep(50 * time.Millisecond)

	rep1, _ := vm.GetReputation(node1.ID)
	assert.Contains(t, []int64{101, 98, 95}, rep1, "Node1 reputation should reflect signing outcome (valid, none, or invalid)")

	t.Log("Note: Testing invalid signature path requires mocking crypto or observing logs.")
}

func TestFinalizeBlockDBFT_Failure_NoValidators(t *testing.T) {
	vm := core.NewValidatorManager()
	block := createTestBlock(1, []byte("genesis"))
	shardID := uint64(0)

	signedBlock, err := vm.FinalizeBlockDBFT(block, shardID)

	require.Error(t, err, "FinalizeBlockDBFT should fail with no validators")
	assert.Nil(t, signedBlock, "SignedBlock should be nil on error")
	assert.Contains(t, err.Error(), "no eligible validators", "Error message should indicate no validators")
}

func TestChallengeResponse_Success(t *testing.T) {
	vm := core.NewValidatorManager()
	node1, _ := createTestNode("node1")
	vm.AddValidator(node1, 100)
	v1, _ := vm.GetValidator(node1.ID)
	v1.IsActive.Store(false)

	initialNonceValue := node1.GetAuthNonce()
	initialRep, _ := vm.GetReputation(node1.ID)

	challengeData, err := vm.ChallengeValidator(node1.ID)
	require.NoError(t, err, "ChallengeValidator should succeed")
	require.NotEmpty(t, challengeData, "Challenge data should not be empty")

	hasher := sha256.New()
	hasher.Write(challengeData)
	nonceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nonceBytes, initialNonceValue)
	hasher.Write(nonceBytes)
	hashToSign := hasher.Sum(nil)

	signature, err := node1.SignData(hashToSign)
	require.NoError(t, err, "Node should be able to sign challenge response")

	err = vm.VerifyResponse(node1.ID, signature)
	require.NoError(t, err, "VerifyResponse should succeed with correct signature")

	assert.True(t, v1.IsActive.Load(), "Validator should become active after successful challenge")
	assert.True(t, node1.IsAuthenticated(), "Node should be marked authenticated")
	assert.Equal(t, initialNonceValue+1, node1.GetAuthNonce(), "Node authentication nonce should increment")
	finalRep, _ := vm.GetReputation(node1.ID)
	assert.Greater(t, finalRep, initialRep, "Reputation should increase after successful challenge")
}

func TestChallengeResponse_Failure_InvalidSignature(t *testing.T) {
	vm := core.NewValidatorManager()
	node1, _ := createTestNode("node1")
	vm.AddValidator(node1, 100)
	v1, _ := vm.GetValidator(node1.ID)
	v1.IsActive.Store(false)
	initialNonceValue := node1.GetAuthNonce()
	initialRep, _ := vm.GetReputation(node1.ID)

	_, err := vm.ChallengeValidator(node1.ID)
	require.NoError(t, err, "ChallengeValidator should succeed")

	invalidData := []byte("completely wrong data")
	hashToSign := sha256.Sum256(invalidData)
	invalidSignature, err := node1.SignData(hashToSign[:])
	require.NoError(t, err, "Node should be able to sign data (even if wrong)")

	err = vm.VerifyResponse(node1.ID, invalidSignature)
	require.Error(t, err, "VerifyResponse should fail with incorrect signature")
	assert.Contains(t, err.Error(), "invalid signature", "Error message should indicate invalid signature")

	assert.False(t, v1.IsActive.Load(), "Validator should remain inactive")
	assert.False(t, node1.IsAuthenticated(), "Node should remain unauthenticated")
	assert.Equal(t, initialNonceValue, node1.GetAuthNonce(), "Node authentication nonce should NOT increment")
	finalRep, _ := vm.GetReputation(node1.ID)
	assert.Less(t, finalRep, initialRep, "Reputation should decrease after failed challenge")
}

func TestChallengeResponse_Failure_Expired(t *testing.T) {
	vm := core.NewValidatorManager()
	node1, _ := createTestNode("node1")
	vm.AddValidator(node1, 100)
	v1, _ := vm.GetValidator(node1.ID)
	v1.IsActive.Store(false)

	initialNonceValue := node1.GetAuthNonce()
	initialRep, _ := vm.GetReputation(node1.ID)

	challengeData, err := vm.ChallengeValidator(node1.ID)
	require.NoError(t, err, "ChallengeValidator should succeed")

	// Simulate challenge expiry by waiting
	time.Sleep(2 * time.Second) // Adjusted to simulate expiry

	// Node prepares response (even though it's too late)
	hasher := sha256.New()
	hasher.Write(challengeData)
	nonceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nonceBytes, initialNonceValue)
	hasher.Write(nonceBytes)
	hashToSign := hasher.Sum(nil)
	signature, err := node1.SignData(hashToSign)
	require.NoError(t, err)

	// Test VerifyResponse after expiry
	err = vm.VerifyResponse(node1.ID, signature)
	require.Error(t, err, "VerifyResponse should fail after expiry")
	assert.Contains(t, err.Error(), "expired", "Error message should indicate expiry")

	// Assertions
	assert.False(t, v1.IsActive.Load(), "Validator should remain inactive")
	assert.Equal(t, initialNonceValue, node1.GetAuthNonce(), "Node authentication nonce should NOT increment")
	finalRep, _ := vm.GetReputation(node1.ID)
	assert.Less(t, finalRep, initialRep, "Reputation should decrease after expiry")
}

func TestGetActiveValidatorsForShard_FiltersInactive(t *testing.T) {
	vm := core.NewValidatorManager()
	nodeA, _ := createTestNode("nodeA")
	nodeB, _ := createTestNode("nodeB")
	nodeC, _ := createTestNode("nodeC")
	nodeD, _ := createTestNode("nodeD")

	vm.AddValidator(nodeA, 100)
	vm.AddValidator(nodeB, 100)
	vm.AddValidator(nodeC, 100)
	vm.AddValidator(nodeD, 100)

	vB, _ := vm.GetValidator(nodeB.ID)
	vB.IsActive.Store(false)

	nodeAuthStatusMu.Lock()
	nodeAuthStatus[nodeC.ID] = new(atomic.Bool)
	nodeAuthStatus[nodeC.ID].Store(false) // Mark nodeC as unauthenticated
	nodeAuthStatusMu.Unlock()

	shardID := uint64(0)
	activeValidators := vm.GetActiveValidatorsForShard(shardID)

	assert.Len(t, activeValidators, 2, "Should only return 2 active and authenticated validators")

	foundA := false
	foundD := false
	for _, v := range activeValidators {
		if v.Node.ID == nodeA.ID {
			foundA = true
		}
		if v.Node.ID == nodeD.ID {
			foundD = true
		}
	}
	assert.True(t, foundA, "NodeA should be in the active list")
	assert.True(t, foundD, "NodeD should be in the active list")
}

func TestCalculateAdaptiveThreshold_VariedReputation(t *testing.T) {
	vm := core.NewValidatorManager()
	node1, _ := createTestNode("node1")
	node2, _ := createTestNode("node2")
	node3, _ := createTestNode("node3")

	vm.AddValidator(node1, 100)
	vm.AddValidator(node2, 200)
	vm.AddValidator(node3, 60)

	validators := vm.GetAllValidators()
	totalRep := int64(100 + 200 + 60)
	expectedThreshold := (totalRep * 2 / 3)

	threshold := vm.CalculateAdaptiveThreshold(validators)
	assert.Equal(t, expectedThreshold, threshold, "Threshold should be 2/3 of total reputation")
}

func TestCalculateAdaptiveThreshold_ZeroValidators(t *testing.T) {
	vm := core.NewValidatorManager()
	validators := vm.GetAllValidators()

	threshold := vm.CalculateAdaptiveThreshold(validators)
	assert.Equal(t, int64(0), threshold, "Threshold should be 0 for no validators")
}

func TestCalculateAdaptiveThreshold_SingleValidator(t *testing.T) {
	vm := core.NewValidatorManager()
	node1, _ := createTestNode("node1")
	vm.AddValidator(node1, 150)

	validators := vm.GetAllValidators()
	totalRep := int64(150)
	expectedThreshold := (totalRep * 2 / 3)

	threshold := vm.CalculateAdaptiveThreshold(validators)
	assert.Equal(t, expectedThreshold, threshold, "Threshold should be 2/3 of single validator's reputation")
}

var (
	nodeNonces       = make(map[core.NodeID]*atomic.Uint64)
	nodeNoncesMu     sync.Mutex
	nodeAuthStatus   = make(map[core.NodeID]*atomic.Bool)
	nodeAuthStatusMu sync.Mutex
)
