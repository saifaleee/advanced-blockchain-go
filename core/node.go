package core

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"sync"
	"sync/atomic"
)

// NodeID represents a unique identifier for a node.
type NodeID string

// GenerateNodeID creates a random NodeID.
func GenerateNodeID() NodeID {
	bytes := make([]byte, 16) // 128 bits
	_, _ = rand.Read(bytes)
	return NodeID(hex.EncodeToString(bytes))
}

// Node represents a participant in the network.
type Node struct {
	ID              NodeID
	PublicKey       []byte      // Placeholder for cryptographic public key
	IsAuthenticated atomic.Bool // Simple flag for authentication status
	// Add other node-specific details like network address if needed
}

// NewNode creates a new basic node.
func NewNode() *Node {
	id := GenerateNodeID()
	// Generate placeholder key
	pk := make([]byte, 32)
	_, _ = rand.Read(pk)

	node := &Node{
		ID:        id,
		PublicKey: pk,
	}
	node.IsAuthenticated.Store(false) // Nodes need to authenticate
	return node
}

// Authenticate marks the node as authenticated (simple simulation).
func (n *Node) Authenticate() {
	n.IsAuthenticated.Store(true)
	log.Printf("Node %s authenticated.", n.ID)
}

// Validator represents a node that participates in consensus (dBFT).
type Validator struct {
	Node       *Node
	Reputation atomic.Int64 // Reputation score
	IsActive   atomic.Bool  // Whether the validator is currently active
	mu         sync.RWMutex // Protects validator-specific state if needed later
}

// NewValidator creates a validator from a node.
func NewValidator(node *Node, initialReputation int64) *Validator {
	v := &Validator{
		Node: node,
	}
	v.Reputation.Store(initialReputation)
	v.IsActive.Store(true) // Assume active initially
	return v
}

// UpdateReputation changes the validator's reputation score.
func (v *Validator) UpdateReputation(change int64) {
	current := v.Reputation.Add(change)
	log.Printf("Validator %s reputation updated by %d to %d", v.Node.ID, change, current)
	// Add logic here to deactivate if reputation drops too low
	// if current < -10 { // Example threshold
	//  v.IsActive.Store(false)
	//  log.Printf("Validator %s deactivated due to low reputation (%d)", v.Node.ID, current)
	// }
}
