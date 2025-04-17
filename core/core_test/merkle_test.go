package core_test

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/saifaleee/advanced-blockchain-go/core"
)

func TestMerkleTree(t *testing.T) {
	// Test Case 1: Even number of transactions
	tx1 := core.NewTransaction([]byte("tx1"))
	tx2 := core.NewTransaction([]byte("tx2"))
	tx3 := core.NewTransaction([]byte("tx3"))
	tx4 := core.NewTransaction([]byte("tx4"))
	txsEven := []*core.Transaction{tx1, tx2, tx3, tx4}
	txIDsEven := core.GetTransactionIDs(txsEven)

	treeEven := core.NewMerkleTree(txIDsEven)
	if treeEven == nil || treeEven.RootNode == nil {
		t.Fatal("Failed to create Merkle tree for even number of transactions")
	}

	// Manually calculate expected root for 4 items
	h1 := sha256.Sum256(tx1.ID)
	h2 := sha256.Sum256(tx2.ID)
	h3 := sha256.Sum256(tx3.ID)
	h4 := sha256.Sum256(tx4.ID)
	h12 := sha256.Sum256(append(h1[:], h2[:]...))
	h34 := sha256.Sum256(append(h3[:], h4[:]...))
	h1234 := sha256.Sum256(append(h12[:], h34[:]...))
	expectedRootEven := h1234[:]

	if !bytes.Equal(treeEven.RootNode.Data, expectedRootEven) {
		t.Errorf("Merkle root mismatch (even): expected %x, got %x", expectedRootEven, treeEven.RootNode.Data)
	}

	// Test Case 2: Odd number of transactions
	tx5 := core.NewTransaction([]byte("tx5"))
	txsOdd := []*core.Transaction{tx1, tx2, tx3, tx4, tx5} // 5 transactions
	txIDsOdd := core.GetTransactionIDs(txsOdd)

	treeOdd := core.NewMerkleTree(txIDsOdd)
	if treeOdd == nil || treeOdd.RootNode == nil {
		t.Fatal("Failed to create Merkle tree for odd number of transactions")
	}

	// Manually calculate expected root for 5 items (dup tx5)
	h5 := sha256.Sum256(tx5.ID)
	// Leaves: h1, h2, h3, h4, h5, h5(dup)
	// Level 1: h12, h34, h55(dup)
	h55 := sha256.Sum256(append(h5[:], h5[:]...))
	// Level 2: h1234, h55, h55(dup) -> Need to duplicate h55
	h5555 := sha256.Sum256(append(h55[:], h55[:]...))
	// Level 3: h12345555
	h12345555 := sha256.Sum256(append(h1234[:], h5555[:]...))
	expectedRootOdd := h12345555[:]

	if !bytes.Equal(treeOdd.RootNode.Data, expectedRootOdd) {
		t.Errorf("Merkle root mismatch (odd): expected %x, got %x", expectedRootOdd, treeOdd.RootNode.Data)
	}

	// Test Case 3: Single transaction
	txsSingle := []*core.Transaction{tx1}
	txIDsSingle := core.GetTransactionIDs(txsSingle)
	treeSingle := core.NewMerkleTree(txIDsSingle)
	if treeSingle == nil || treeSingle.RootNode == nil {
		t.Fatal("Failed to create Merkle tree for single transaction")
	}
	expectedRootSingle := sha256.Sum256(tx1.ID)
	if !bytes.Equal(treeSingle.RootNode.Data, expectedRootSingle[:]) {
		t.Errorf("Merkle root mismatch (single): expected %x, got %x", expectedRootSingle, treeSingle.RootNode.Data)
	}

	// Test Case 4: No transactions
	txsEmpty := []*core.Transaction{}
	txIDsEmpty := core.GetTransactionIDs(txsEmpty)
	treeEmpty := core.NewMerkleTree(txIDsEmpty)
	if treeEmpty == nil || treeEmpty.RootNode == nil {
		t.Fatal("Failed to create Merkle tree for empty transaction list")
	}
	expectedEmptyRoot := sha256.Sum256([]byte{})
	if !bytes.Equal(treeEmpty.RootNode.Data, expectedEmptyRoot[:]) {
		t.Errorf("Merkle root mismatch (empty): expected %x, got %x", expectedEmptyRoot[:], treeEmpty.RootNode.Data)
	}
}
