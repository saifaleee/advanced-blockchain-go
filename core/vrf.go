package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort" // Added for sorting NodeIDs
)

// VRFOutput represents the output and proof of a VRF evaluation.
type VRFOutput struct {
	Input []byte
	Value uint64 // Pseudo-random value derived from input
	Proof []byte // Placeholder for cryptographic proof (e.g., the hash itself)
}

// SimpleVRF is a placeholder implementation. NOT SECURE.
type SimpleVRF struct {
	// In a real VRF, this would hold the secret key.
	// We simulate its effect without storing it explicitly here.
	seed []byte // Only used for generation simulation
}

func NewSimpleVRF(seed []byte) *SimpleVRF {
	// Seed is used only conceptually for generation simulation
	// Store a copy of the seed to prevent external modification
	seedCopy := make([]byte, len(seed))
	copy(seedCopy, seed)
	return &SimpleVRF{seed: seedCopy}
}

// Evaluate simulates computing the VRF output using a secret key (represented by seed).
func (v *SimpleVRF) Evaluate(input []byte) (*VRFOutput, error) {
	if v.seed == nil {
		// This check might be removed if seed is only conceptual,
		// but useful for ensuring the generator is initialized.
		return nil, fmt.Errorf("VRF seed not initialized for evaluation simulation")
	}
	// Simulate using a secret (seed) + input to generate hash/proof
	data := append(append([]byte{}, v.seed...), input...) // Use seed copy
	hash := sha256.Sum256(data)
	proof := hash[:] // Proof is the hash in this simulation

	// Derive value from the proof (publicly derivable part)
	value := binary.BigEndian.Uint64(proof[:8])

	// Return a copy of the input in the output
	inputCopy := make([]byte, len(input))
	copy(inputCopy, input)

	return &VRFOutput{
		Input: inputCopy,
		Value: value,
		Proof: proof,
	}, nil
}

// VerifyVRF simulates public verification using only public information (input, output).
func VerifyVRF(input []byte, output *VRFOutput) bool {
	if output == nil {
		// fmt.Println("VRF Verify Fail: Output nil") // Reduce log noise
		return false
	}
	if !bytes.Equal(input, output.Input) {
		// fmt.Printf("VRF Verify Fail: Input mismatch (Expected: %x, Got: %x)", input, output.Input) // Reduce log noise
		return false
	}
	// Check if proof has the expected format (SHA256 hash length)
	if len(output.Proof) != sha256.Size {
		// fmt.Printf("VRF Verify Fail: Proof length %d != %d", len(output.Proof), sha256.Size) // Reduce log noise
		return false
	}
	// Check if the value can be derived from the proof (as per our placeholder logic)
	expectedValue := binary.BigEndian.Uint64(output.Proof[:8])
	if output.Value != expectedValue {
		// fmt.Printf("VRF Verify Fail: Value mismatch (Output: %d, Expected from Proof: %d)", output.Value, expectedValue) // Reduce log noise
		return false
	}

	// In a real VRF, verification would involve cryptographic checks using a public key.
	// This placeholder check is very basic.
	return true
}

// Helper function to create input for VRF based on conflicting items
// Takes key, sorted list of NodeIDs, and additional context.
func CreateVRFInput(key string, conflictingNodeIDs []NodeID, context []byte) []byte {
	var parts [][]byte
	parts = append(parts, []byte(key))
	// Node IDs should already be sorted before calling this helper
	for _, id := range conflictingNodeIDs {
		parts = append(parts, []byte(id))
	}
	if context != nil {
		parts = append(parts, context)
	}
	return bytes.Join(parts, []byte{0}) // Use null byte as separator
}

// Helper to sort NodeIDs deterministically
func SortNodeIDs(nodeIDs []NodeID) {
	sort.Slice(nodeIDs, func(i, j int) bool {
		return nodeIDs[i] < nodeIDs[j]
	})
}
