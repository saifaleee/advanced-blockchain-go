package core

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
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

// NewSecureVRF generates a new SecureVRF instance with a fresh ECDSA key pair.
func NewSecureVRF() (*SecureVRF, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ECDSA key pair: %w", err)
	}
	return &SecureVRF{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
	}, nil
}

// Evaluate computes the VRF output and proof for a given input.
func (v *SecureVRF) Evaluate(input []byte) (*VRFOutput, error) {
	hash := sha256.Sum256(input)
	r, s, err := ecdsa.Sign(rand.Reader, v.privateKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign input: %w", err)
	}

	proof := append(r.Bytes(), s.Bytes()...)
	return &VRFOutput{
		Output: hash[:],
		Proof:  proof,
	}, nil
}

// Verify verifies the VRF output and proof using the public key.
func (v *SecureVRF) Verify(input []byte, output *VRFOutput) bool {
	hash := sha256.Sum256(input)
	if !equal(hash[:], output.Output) {
		return false
	}

	r := new(big.Int).SetBytes(output.Proof[:len(output.Proof)/2])
	s := new(big.Int).SetBytes(output.Proof[len(output.Proof)/2:])
	return ecdsa.Verify(v.publicKey, hash[:], r, s)
}

// VerifyProof verifies the VRF proof for a given input and output.
func (v *SecureVRF) VerifyProof(input []byte, output *VRFOutput) bool {
	hash := sha256.Sum256(input)
	if !bytes.Equal(hash[:], output.Output) {
		return false
	}

	r := new(big.Int).SetBytes(output.Proof[:len(output.Proof)/2])
	s := new(big.Int).SetBytes(output.Proof[len(output.Proof)/2:])
	return ecdsa.Verify(v.publicKey, hash[:], r, s)
}

// SelectProposer selects a proposer using VRF.
func (v *SecureVRF) SelectProposer(validators []*Validator, seed []byte) (*Validator, error) {
	var bestValidator *Validator
	var lowestValue *big.Int

	for _, validator := range validators {
		input := append(seed, []byte(validator.Node.ID)...)
		output, err := v.Evaluate(input)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate VRF: %w", err)
		}

		value := new(big.Int).SetBytes(output.Output)
		if lowestValue == nil || value.Cmp(lowestValue) < 0 {
			lowestValue = value
			bestValidator = validator
		}
	}

	return bestValidator, nil
}

// SelectProposerWithProof selects a proposer using VRF and returns the proof for verification.
func (v *SecureVRF) SelectProposerWithProof(validators []*Validator, seed []byte) (*Validator, *VRFOutput, error) {
	var bestValidator *Validator
	var lowestValue *big.Int
	var bestProof *VRFOutput

	for _, validator := range validators {
		input := append(seed, []byte(validator.Node.ID)...)
		output, err := v.Evaluate(input)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to evaluate VRF: %w", err)
		}

		value := new(big.Int).SetBytes(output.Output)
		if lowestValue == nil || value.Cmp(lowestValue) < 0 {
			lowestValue = value
			bestValidator = validator
			bestProof = output
		}
	}

	return bestValidator, bestProof, nil
}

// SelectDelegateWithVRF selects a delegate using VRF for consensus.
func (v *SecureVRF) SelectDelegateWithVRF(seed []byte, validators []*Validator) (*Validator, error) {
	var bestValidator *Validator
	var lowestValue *big.Int

	for _, validator := range validators {
		input := append(seed, []byte(validator.Node.ID)...)
		output, err := v.Evaluate(input)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate VRF: %w", err)
		}

		value := new(big.Int).SetBytes(output.Output)
		if lowestValue == nil || value.Cmp(lowestValue) < 0 {
			lowestValue = value
			bestValidator = validator
		}
	}

	return bestValidator, nil
}

// IntegrateVRFIntoConsensus integrates VRF into the consensus process.
func (v *SecureVRF) IntegrateVRFIntoConsensus(seed []byte, validators []*Validator) (*Validator, *VRFOutput, error) {
	validator, proof, err := v.SelectProposerWithProof(validators, seed)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to integrate VRF into consensus: %w", err)
	}
	return validator, proof, nil
}

// equal compares two byte slices for equality.
func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
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

// GenerateAccumulatorProof generates a proof for a validator's participation.
func GenerateAccumulatorProof(validator *Validator, seed []byte) ([]byte, error) {
	hash := sha256.Sum256(append(seed, []byte(validator.Node.ID)...))
	return hash[:], nil
}

// VerifyAccumulatorProof verifies the proof for a validator's participation.
func VerifyAccumulatorProof(validator *Validator, seed []byte, proof []byte) bool {
	expectedHash := sha256.Sum256(append(seed, []byte(validator.Node.ID)...))
	return bytes.Equal(expectedHash[:], proof)
}
