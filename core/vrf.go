package core

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"

	// "crypto/elliptic" // No longer generating keys here
	// "crypto/rand" // No longer generating keys here
	"crypto/sha256"
	"fmt"
	"log"
	"math/big"
	"sort"
)

// VRFOutput represents the output and proof of a VRF evaluation.
type VRFOutput struct {
	Output []byte
	Proof  []byte
}

// SecureVRF implements a secure VRF using ECDSA keys.
type SecureVRF struct {
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
}

// --- FIX: Modify NewSecureVRF to accept existing keys ---
// NewSecureVRF creates a new SecureVRF instance using existing ECDSA keys.
func NewSecureVRF(privKey *ecdsa.PrivateKey, pubKey *ecdsa.PublicKey) (*SecureVRF, error) {
	if privKey == nil || pubKey == nil {
		return nil, fmt.Errorf("private and public keys cannot be nil")
	}
	// Optional: Add check to ensure keys match? pubKey == &privKey.PublicKey
	if pubKey.X.Cmp(privKey.PublicKey.X) != 0 || pubKey.Y.Cmp(privKey.PublicKey.Y) != 0 {
		return nil, fmt.Errorf("provided private and public keys do not match")
	}
	return &SecureVRF{
		privateKey: privKey,
		publicKey:  pubKey,
	}, nil
}

// --- END FIX ---

// Evaluate computes the VRF output and proof for a given input *using this VRF instance's private key*.
func (v *SecureVRF) Evaluate(input []byte) (*VRFOutput, error) {
	// Check if keys are initialized
	if v.privateKey == nil || v.publicKey == nil {
		return nil, fmt.Errorf("VRF keys not initialized")
	}
	// log.Printf("[VRF] Evaluating input: %x", input)
	// log.Printf("[VRF Debug] Evaluating with Public Key: X=%x, Y=%x", v.publicKey.X.Bytes(), v.publicKey.Y.Bytes())
	hash := sha256.Sum256(input)
	r, s, err := ecdsa.Sign(rand.Reader, v.privateKey, hash[:]) // Use crypto/rand explicitly
	if err != nil {
		log.Printf("[VRF] Error signing input: %v", err)
		return nil, fmt.Errorf("failed to sign input: %w", err)
	}

	// Ensure r and s are properly padded to 32 bytes for a 64-byte proof
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	// P256 components should fit within 32 bytes. Check just in case.
	if len(rBytes) > 32 || len(sBytes) > 32 {
		log.Printf("[VRF] Invalid signature component length: r=%d, s=%d", len(rBytes), len(sBytes))
		return nil, fmt.Errorf("invalid signature component length (r=%d, s=%d)", len(rBytes), len(sBytes))
	}

	// Construct the 64-byte proof: 32 bytes for R, 32 bytes for S
	proof := make([]byte, 64)
	copy(proof[32-len(rBytes):32], rBytes) // Pad R to the left
	copy(proof[64-len(sBytes):], sBytes)   // Pad S to the left

	// log.Printf("[VRF] Generated proof: %x, Output: %x", proof, hash[:])
	return &VRFOutput{
		Output: hash[:], // VRF output is the hash itself in this construction
		Proof:  proof,   // Proof is the concatenated (r, s) signature
	}, nil
}

// ... (rest of vrf.go remains the same as previous fix)
// VerifyWithPublicKey, CreateVRFInput, SortNodeIDs, GenerateAccumulatorProof, VerifyAccumulatorProof

// VerifyWithPublicKey verifies the VRF output and proof using the provided public key.
// It checks that the output matches the hash of the input, and that the proof is a valid ECDSA signature of that hash.
func VerifyWithPublicKey(publicKey *ecdsa.PublicKey, input []byte, vrfOutput *VRFOutput) bool {
	if vrfOutput == nil || publicKey == nil {
		log.Println("[VRF Verify] Error: nil public key or VRF output provided.")
		return false
	}
	// log.Printf("[VRF] Verifying input: %x, Output: %x, Proof: %x", input, vrfOutput.Output, vrfOutput.Proof)
	// log.Printf("[VRF Debug] Verifying with Public Key: X=%x, Y=%x", publicKey.X.Bytes(), publicKey.Y.Bytes())

	// 1. Verify Output matches Hash(Input)
	expectedHash := sha256.Sum256(input)
	if !bytes.Equal(expectedHash[:], vrfOutput.Output) {
		log.Printf("[VRF Verify] Output mismatch. Expected Hash(Input): %x, Got Output: %x", expectedHash[:], vrfOutput.Output)
		return false
	}

	// 2. Verify Proof is a valid signature of Hash(Input)
	// Validate proof length (expecting 64 bytes for P256: 32 for r, 32 for s)
	if len(vrfOutput.Proof) != 64 {
		log.Printf("[VRF Verify] Proof length invalid. Expected: 64, Got: %d", len(vrfOutput.Proof))
		return false
	}

	// Extract r and s from the proof
	r := new(big.Int).SetBytes(vrfOutput.Proof[:32])
	s := new(big.Int).SetBytes(vrfOutput.Proof[32:])
	// log.Printf("[VRF Debug] Extracted r: %x, s: %x", r.Bytes(), s.Bytes())

	// Use ecdsa.Verify to check the signature (proof) against the hash (output)
	isValid := ecdsa.Verify(publicKey, vrfOutput.Output, r, s) // Verify against the Output which should be hash(input)
	if !isValid {
		log.Printf("[VRF Verify] ECDSA verification failed. Hash(Input)=Output: %x, r: %x, s: %x", vrfOutput.Output, r.Bytes(), s.Bytes())
		return false
	}

	// log.Printf("[VRF Verify] Proof verification succeeded.")
	return true
}

// --- REMOVED incorrect/unused methods ---
/*
// VerifyProof verifies the VRF proof for a given input and output using the provided public key.
// Deprecated: Use VerifyWithPublicKey for clarity and consistency.
func VerifyProofWithPublicKey(publicKey *ecdsa.PublicKey, input []byte, output *VRFOutput) bool {
	// ... implementation ... // REMOVED
}

// SelectProposer selects a proposer using VRF *using the current VRF instance's key, NOT validator keys*. Likely incorrect.
func (v *SecureVRF) SelectProposer(validators []*Validator, seed []byte) (*Validator, error) {
	// ... implementation ... // REMOVED
}

// SelectProposerWithProof selects a proposer using VRF *using the current VRF instance's key*. Likely incorrect.
func (v *SecureVRF) SelectProposerWithProof(validators []*Validator, seed []byte) (*Validator, *VRFOutput, error) {
	// ... implementation ... // REMOVED
}

// SelectDelegateWithVRF selects a delegate using VRF *using the current VRF instance's key*. Likely incorrect.
func (v *SecureVRF) SelectDelegateWithVRF(seed []byte, validators []*Validator) (*Validator, error) {
	// ... implementation ... // REMOVED
}
*/
// --- END REMOVED methods ---

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

// GenerateAccumulatorProof generates a proof for a validator's participation (Simplified Example).
func GenerateAccumulatorProof(validator *Validator, seed []byte) ([]byte, error) {
	if validator == nil || validator.Node == nil {
		return nil, fmt.Errorf("invalid validator provided")
	}
	hash := sha256.Sum256(append(seed, []byte(validator.Node.ID)...))
	return hash[:], nil
}

// VerifyAccumulatorProof verifies the proof for a validator's participation (Simplified Example).
func VerifyAccumulatorProof(validator *Validator, seed []byte, proof []byte) bool {
	if validator == nil || validator.Node == nil {
		return false
	}
	expectedHash := sha256.Sum256(append(seed, []byte(validator.Node.ID)...))
	return bytes.Equal(expectedHash[:], proof)
}
