package core

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync/atomic"
	"time"
)

// Node represents a participant in the network.
type Node struct {
	ID             NodeID
	PrivateKey     *ecdsa.PrivateKey
	PublicKey      *ecdsa.PublicKey
	Authentication *NodeAuthentication // Changed to pointer for atomic updates
	TrustScore     float64             // Adaptive trust score
	LastAttested   time.Time           // Timestamp of last successful attestation/challenge response
}

// NodeAuthentication holds authentication-related state for a node.
// Using atomic values for thread-safe updates.
type NodeAuthentication struct {
	IsAuthenticated atomic.Bool   // Use atomic.Bool
	AuthNonce       atomic.Uint64 // Use atomic.Uint64 for nonce
}

// NewNode creates a new Node with ECDSA keys.
// Returns Node and error.
func NewNode(id NodeID) (*Node, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ECDSA key for node %s: %w", id, err)
	}
	node := &Node{
		ID:             id,
		PrivateKey:     privateKey,
		PublicKey:      &privateKey.PublicKey,
		Authentication: &NodeAuthentication{}, // Initialize authentication struct
		TrustScore:     0.5,                   // Initial neutral trust score
	}
	// Initialize atomic values
	node.Authentication.IsAuthenticated.Store(false) // Start as not authenticated
	node.Authentication.AuthNonce.Store(0)
	return node, nil
}

// SignData signs arbitrary data using the node's private key.
// Assumes the input data is the hash that needs to be signed.
func (n *Node) SignData(dataHash []byte) ([]byte, error) {
	signature, err := ecdsa.SignASN1(rand.Reader, n.PrivateKey, dataHash) // Use dataHash directly
	if err != nil {
		return nil, fmt.Errorf("node %s failed to sign data: %w", n.ID, err)
	}
	return signature, nil
}

// VerifySignature verifies a signature against the node's public key.
// Assumes the input data is the hash that was signed.
func (n *Node) VerifySignature(dataHash []byte, signature []byte) bool {
	return ecdsa.VerifyASN1(n.PublicKey, dataHash, signature) // Use dataHash directly
}

// PublicKeyHex returns the public key as a hex string.
func (n *Node) PublicKeyHex() string {
	return hex.EncodeToString(elliptic.Marshal(n.PublicKey.Curve, n.PublicKey.X, n.PublicKey.Y))
}

// Authenticate marks the node as authenticated (simplified).
func (n *Node) Authenticate() {
	n.Authentication.IsAuthenticated.Store(true)
	n.LastAttested = time.Now() // Update attestation time
	n.TrustScore = 0.75         // Increase trust on basic auth
	fmt.Printf("[Auth] Node %s basic authentication successful.\n", n.ID)
}

// IsAuthenticated checks if the node is currently authenticated.
func (n *Node) IsAuthenticated() bool {
	return n.Authentication.IsAuthenticated.Load()
}

// GetAuthNonce gets the current authentication nonce.
func (n *Node) GetAuthNonce() uint64 {
	return n.Authentication.AuthNonce.Load()
}

// IncrementAuthNonce increments the authentication nonce atomically.
func (n *Node) IncrementAuthNonce() uint64 {
	return n.Authentication.AuthNonce.Add(1)
}

// UpdateTrustScore adjusts the node's trust score based on behavior.
func (n *Node) UpdateTrustScore(change float64) {
	n.TrustScore += change
	// Clamp score between 0 and 1
	if n.TrustScore > 1.0 {
		n.TrustScore = 1.0
	} else if n.TrustScore < 0.0 {
		n.TrustScore = 0.0
	}
	fmt.Printf("[Auth] Node %s trust score updated to %.2f\n", n.ID, n.TrustScore)
}

// PenalizeTrustScore decreases the node's trust score for adversarial behavior.
func (n *Node) PenalizeTrustScore(amount float64) {
	n.TrustScore -= amount
	if n.TrustScore < 0.0 {
		n.TrustScore = 0.0
	}
	fmt.Printf("[Adversarial] Node %s trust score penalized by %.2f. New trust score: %.2f\n", n.ID, amount, n.TrustScore)
}

// RewardTrustScore increases the node's trust score for positive behavior.
func (n *Node) RewardTrustScore(amount float64) {
	n.TrustScore += amount
	if n.TrustScore > 1.0 {
		n.TrustScore = 1.0
	}
	fmt.Printf("[Trust] Node %s trust score rewarded by %.2f. New trust score: %.2f\n", n.ID, amount, n.TrustScore)
}

// Add adaptive trust scoring logic to Node
func (n *Node) AdjustTrustScoreBasedOnBehavior(positive bool) {
	if positive {
		n.TrustScore += 0.1
		if n.TrustScore > 1.0 {
			n.TrustScore = 1.0
		}
	} else {
		n.TrustScore -= 0.2
		if n.TrustScore < 0.0 {
			n.TrustScore = 0.0
		}
	}
	log.Printf("[Trust] Node %s trust score adjusted to %.2f", n.ID, n.TrustScore)
}

// Attest performs a challenge-response attestation for the node.
func (n *Node) Attest(challenge []byte) ([]byte, error) {
	signature, err := n.SignData(challenge)
	if err != nil {
		return nil, fmt.Errorf("failed to attest challenge: %w", err)
	}
	n.LastAttested = time.Now()
	return signature, nil
}

// VerifyAttestation verifies the attestation response.
func (n *Node) VerifyAttestation(challenge, response []byte) bool {
	return n.VerifySignature(challenge, response)
}

// --- Validator --- //

// Validator extends Node with reputation and active status for consensus.
type Validator struct {
	Node       *Node
	Reputation atomic.Int64 // Use atomic.Int64
	IsActive   atomic.Bool  // Use atomic.Bool
}

// NewValidator creates a new Validator instance.
func NewValidator(node *Node, initialReputation int64) *Validator {
	v := &Validator{
		Node: node,
	}
	v.Reputation.Store(initialReputation)
	v.IsActive.Store(true) // Assume active initially
	return v
}
