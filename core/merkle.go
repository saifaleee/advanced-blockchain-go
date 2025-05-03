package core

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
)

// SimpleHash is a basic hashing function used internally.
func SimpleHash(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

// MerkleNode represents a node in the Merkle tree.
type MerkleNode struct {
	Left  *MerkleNode
	Right *MerkleNode
	Data  []byte // Hash value of the node
}

// MerkleTree represents the complete Merkle tree.
type MerkleTree struct {
	RootNode *MerkleNode
	Leaves   []*MerkleNode // Added to easily find leaves for proof generation
}

// MerkleProof contains the path hashes needed to verify inclusion.
type MerkleProof struct {
	Hashes    [][]byte // Sibling hashes along the path from leaf to root
	Index     uint64   // Index of the leaf the proof is for
	NumLeaves uint64   // Total number of leaves when proof was generated
}

// NewMerkleNode creates a new Merkle tree node.
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	node := MerkleNode{}

	if left == nil && right == nil {
		// Leaf node
		node.Data = SimpleHash(data)
	} else {
		// Internal node
		prevHashes := append(left.Data, right.Data...)
		node.Data = SimpleHash(prevHashes)
	}

	node.Left = left
	node.Right = right

	return &node
}

// NewMerkleTree creates a Merkle tree from a slice of data (e.g., transaction hashes).
func NewMerkleTree(data [][]byte) (*MerkleTree, error) {
	if len(data) == 0 {
		// Handle empty data case - return a tree with a nil root or predefined empty hash?
		// Let's return an error or a tree with a specific empty root representation.
		// Using an empty root node with a known hash might be better.
		// return nil, errors.New("cannot create Merkle tree with no data")
		// Return a tree with a special "empty" root hash
		emptyRoot := SimpleHash([]byte{})
		rootNode := &MerkleNode{Data: emptyRoot}
		return &MerkleTree{RootNode: rootNode, Leaves: []*MerkleNode{}}, nil
	}

	var nodes []*MerkleNode
	var leaves []*MerkleNode

	// Create leaf nodes
	for _, dat := range data {
		node := NewMerkleNode(nil, nil, dat)
		nodes = append(nodes, node)
		leaves = append(leaves, node) // Store leaves
	}

	// Handle odd number of leaves by duplicating the last one
	if len(nodes)%2 != 0 {
		nodes = append(nodes, nodes[len(nodes)-1])
	}

	// Build the tree level by level
	for len(nodes) > 1 {
		var newLevel []*MerkleNode

		for i := 0; i < len(nodes); i += 2 {
			node := NewMerkleNode(nodes[i], nodes[i+1], nil) // Data is calculated inside NewMerkleNode
			newLevel = append(newLevel, node)
		}

		// Handle odd number of nodes in the current level
		if len(newLevel)%2 != 0 && len(newLevel) > 1 { // Avoid duplicating if only root remains
			newLevel = append(newLevel, newLevel[len(newLevel)-1])
		}
		nodes = newLevel
	}
	if len(nodes) == 0 {
		// Should not happen if initial data was not empty, but handle defensively
		return nil, errors.New("internal error: tree construction resulted in zero nodes")
	}

	tree := MerkleTree{RootNode: nodes[0], Leaves: leaves}
	return &tree, nil
}

// NewMerkleTreeFromRoot initializes a Merkle tree from a given root hash.
func NewMerkleTreeFromRoot(rootHash []byte) *MerkleTree {
	return &MerkleTree{
		RootNode: &MerkleNode{Data: rootHash},
	}
}

// GetMerkleRoot returns the root hash of the tree.
func (t *MerkleTree) GetMerkleRoot() []byte {
	if t == nil || t.RootNode == nil {
		return nil // Or return the predefined empty hash
	}
	return t.RootNode.Data
}

// EmptyMerkleRoot returns a consistent hash for an empty set of transactions/data.
func EmptyMerkleRoot() []byte {
	return SimpleHash([]byte{})
}

// GenerateMerkleProof generates a proof for the data at the given index.
func (t *MerkleTree) GenerateMerkleProof(index uint64) (*MerkleProof, error) {
	numLeaves := uint64(len(t.Leaves))
	if index >= numLeaves {
		return nil, fmt.Errorf("index %d out of bounds for %d leaves", index, numLeaves)
	}

	proof := &MerkleProof{
		Hashes:    [][]byte{},
		Index:     index,
		NumLeaves: numLeaves,
	}

	var currentLevel []*MerkleNode
	// Create initial level from leaves for easier indexing
	for _, leaf := range t.Leaves {
		currentLevel = append(currentLevel, leaf)
	}
	// Handle odd number of leaves for hashing consistency during proof generation
	if len(currentLevel)%2 != 0 {
		currentLevel = append(currentLevel, currentLevel[len(currentLevel)-1])
	}

	nodeIndex := index

	// Iterate up the tree levels (conceptually)
	for len(currentLevel) > 1 {
		var nextLevel []*MerkleNode
		if nodeIndex%2 == 0 {
			// Node is on the left, sibling is on the right
			siblingIndex := nodeIndex + 1
			if siblingIndex < uint64(len(currentLevel)) { // Ensure sibling exists
				proof.Hashes = append(proof.Hashes, currentLevel[siblingIndex].Data)
			} else {
				// Should not happen if duplication logic is correct, but indicates an error
				return nil, fmt.Errorf("internal error: missing sibling node during proof generation at index %d", nodeIndex)
			}

		} else {
			// Node is on the right, sibling is on the left
			siblingIndex := nodeIndex - 1
			proof.Hashes = append(proof.Hashes, currentLevel[siblingIndex].Data)
		}

		// Calculate parent nodes for the next level
		for i := 0; i < len(currentLevel); i += 2 {
			// Combine hashes correctly regardless of original data (use node data)
			combined := append(currentLevel[i].Data, currentLevel[i+1].Data...)
			parentHash := SimpleHash(combined)
			parent := &MerkleNode{Data: parentHash} // No need for full node structure here
			nextLevel = append(nextLevel, parent)
		}
		currentLevel = nextLevel
		// Handle odd number of nodes in the new level
		if len(currentLevel)%2 != 0 && len(currentLevel) > 1 {
			currentLevel = append(currentLevel, currentLevel[len(currentLevel)-1])
		}

		nodeIndex /= 2 // Move to parent index
	}

	return proof, nil
}

// GenerateProof generates a Merkle proof for a specific data item.
func (t *MerkleTree) GenerateProof(data []byte) ([]byte, error) {
	if t.RootNode == nil {
		return nil, fmt.Errorf("cannot generate proof for an empty Merkle tree")
	}

	// Placeholder implementation: Return the root hash as proof for simplicity.
	// Replace this with actual proof generation logic.
	return t.RootNode.Data, nil
}

// VerifyProof verifies a Merkle proof for a specific data item.
func (t *MerkleTree) VerifyProof(data []byte, proof []byte) bool {
	if t.RootNode == nil {
		return false // Cannot verify proof for an empty Merkle tree
	}

	// Placeholder implementation: Simply compare the proof with the root hash.
	// Replace this with actual proof verification logic.
	return bytes.Equal(proof, t.RootNode.Data)
}

// VerifyMerkleProof checks if the provided data belongs to the tree using the proof.
func VerifyMerkleProof(rootHash []byte, leafData []byte, proof *MerkleProof) bool {
	if proof == nil {
		return false
	}

	currentHash := SimpleHash(leafData) // Start with the hash of the leaf data
	currentIndex := proof.Index

	// Handle odd number of total leaves impacting proof verification path
	numLeavesCurrentLevel := proof.NumLeaves
	if numLeavesCurrentLevel == 0 && len(rootHash) == 0 { // Special case: empty tree verification
		return true // Or compare rootHash with EmptyMerkleRoot() if defined
	}
	if numLeavesCurrentLevel == 0 {
		return false // Proof for empty tree but non-empty root
	}

	for _, siblingHash := range proof.Hashes {
		var combined []byte
		if currentIndex%2 == 0 {
			// Current hash is on the left, sibling is on the right
			combined = append(currentHash, siblingHash...)
		} else {
			// Current hash is on the right, sibling is on the left
			combined = append(siblingHash, currentHash...)
		}
		currentHash = SimpleHash(combined)
		currentIndex /= 2

		// Adjust leaf count conceptually for the next level
		if numLeavesCurrentLevel%2 != 0 && numLeavesCurrentLevel > 1 {
			numLeavesCurrentLevel = numLeavesCurrentLevel/2 + 1
		} else {
			numLeavesCurrentLevel /= 2
		}
	}

	// The final hash should match the root hash
	return bytes.Equal(currentHash, rootHash)
}
