package core

import (
	"crypto/sha256"
	"log"
)

// MerkleNode represents a node in the Merkle tree.
type MerkleNode struct {
	Left  *MerkleNode
	Right *MerkleNode
	Data  []byte // Hash data
}

// MerkleTree represents the complete Merkle tree.
type MerkleTree struct {
	RootNode *MerkleNode
}

// NewMerkleNode creates a new Merkle tree node.
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	node := MerkleNode{}

	if left == nil && right == nil {
		// Leaf node: hash the actual data (e.g., transaction ID)
		hash := sha256.Sum256(data)
		node.Data = hash[:]
	} else {
		// Internal node: hash the concatenation of children's hashes
		prevHashes := append(left.Data, right.Data...)
		hash := sha256.Sum256(prevHashes)
		node.Data = hash[:]
	}

	node.Left = left
	node.Right = right

	return &node
}

// NewMerkleTree creates a Merkle tree from a slice of data (e.g., transaction IDs).
func NewMerkleTree(data [][]byte) *MerkleTree {
	if len(data) == 0 {
		log.Println("Warning: Creating Merkle tree with no data.")
		// Return a tree with a predefined empty hash or handle as needed
		hash := sha256.Sum256([]byte{})
		root := MerkleNode{Data: hash[:]}
		return &MerkleTree{&root}
	}

	var nodes []MerkleNode

	// Create leaf nodes
	for _, datum := range data {
		node := NewMerkleNode(nil, nil, datum)
		nodes = append(nodes, *node)
	}

	// Handle case where there are no transactions after filtering invalid ones etc.
	// It should ideally not happen if block creation requires txs, but defensive coding helps.
	if len(nodes) == 0 {
		log.Println("Warning: No valid leaf nodes to build Merkle tree.")
		hash := sha256.Sum256([]byte{})
		root := MerkleNode{Data: hash[:]}
		return &MerkleTree{&root}
	}

	// Build the tree level by level
	for len(nodes) > 1 {
		// If odd number of nodes, duplicate the last one
		if len(nodes)%2 != 0 {
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		var level []MerkleNode
		for i := 0; i < len(nodes); i += 2 {
			node := NewMerkleNode(&nodes[i], &nodes[i+1], nil)
			level = append(level, *node)
		}
		nodes = level
	}

	// The single remaining node is the root
	tree := MerkleTree{&nodes[0]}
	return &tree
}

// Helper function to get Transaction IDs for Merkle Tree construction
func GetTransactionIDs(transactions []*Transaction) [][]byte {
	var txIDs [][]byte
	for _, tx := range transactions {
		txIDs = append(txIDs, tx.ID)
	}
	return txIDs
}
