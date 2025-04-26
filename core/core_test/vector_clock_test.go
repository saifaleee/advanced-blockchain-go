package core_test

import (
	"testing"

	"github.com/saifaleee/advanced-blockchain-go/core"
	"github.com/stretchr/testify/assert"
)

func TestVectorClock_New(t *testing.T) {
	vc := core.NewVectorClock()
	assert.NotNil(t, vc)
	assert.Empty(t, vc)
}

func TestVectorClock_Increment(t *testing.T) {
	vc := core.NewVectorClock()
	nodeA := core.NodeID("nodeA")
	nodeB := core.NodeID("nodeB")

	vc.Increment(nodeA)
	assert.Equal(t, uint64(1), vc[nodeA])
	assert.Equal(t, uint64(0), vc[nodeB]) // Ensure others are unaffected

	vc.Increment(nodeA)
	assert.Equal(t, uint64(2), vc[nodeA])

	vc.Increment(nodeB)
	assert.Equal(t, uint64(1), vc[nodeB])
	assert.Equal(t, uint64(2), vc[nodeA]) // Ensure others are unaffected
}

func TestVectorClock_Merge(t *testing.T) {
	vc1 := core.NewVectorClock()
	vc2 := core.NewVectorClock()
	nodeA := core.NodeID("nodeA")
	nodeB := core.NodeID("nodeB")
	nodeC := core.NodeID("nodeC")

	vc1[nodeA] = 2
	vc1[nodeB] = 1

	vc2[nodeB] = 3
	vc2[nodeC] = 1

	// Merge vc2 into vc1
	vc1.Merge(vc2)

	assert.Equal(t, uint64(2), vc1[nodeA]) // Unchanged from vc1
	assert.Equal(t, uint64(3), vc1[nodeB]) // Max from vc2
	assert.Equal(t, uint64(1), vc1[nodeC]) // From vc2
}

func TestVectorClock_Merge_Self(t *testing.T) {
	vc := core.NewVectorClock()
	nodeA := core.NodeID("nodeA")
	vc[nodeA] = 5
	vcCopy := vc.Copy()

	vc.Merge(vcCopy)
	assert.Equal(t, uint64(5), vc[nodeA])
	assert.Len(t, vc, 1)
}

func TestVectorClock_Compare_Equal(t *testing.T) {
	vc1 := core.NewVectorClock()
	vc2 := core.NewVectorClock()
	nodeA := core.NodeID("nodeA")
	nodeB := core.NodeID("nodeB")

	vc1[nodeA] = 2
	vc1[nodeB] = 1
	vc2[nodeA] = 2
	vc2[nodeB] = 1

	assert.Equal(t, core.Equal, vc1.Compare(vc2))
	assert.Equal(t, core.Equal, vc2.Compare(vc1))
	assert.False(t, vc1.IsCausallyBefore(vc2))
	assert.False(t, vc2.IsCausallyBefore(vc1))
	assert.False(t, vc1.IsConcurrent(vc2))
}

func TestVectorClock_Compare_BeforeAfter(t *testing.T) {
	vc1 := core.NewVectorClock() // {A:1, B:1}
	vc2 := core.NewVectorClock() // {A:1, B:2}
	nodeA := core.NodeID("nodeA")
	nodeB := core.NodeID("nodeB")

	vc1[nodeA] = 1
	vc1[nodeB] = 1
	vc2[nodeA] = 1
	vc2[nodeB] = 2

	assert.Equal(t, core.Before, vc1.Compare(vc2))
	assert.Equal(t, core.After, vc2.Compare(vc1))
	assert.True(t, vc1.IsCausallyBefore(vc2))
	assert.False(t, vc2.IsCausallyBefore(vc1))
	assert.False(t, vc1.IsConcurrent(vc2))
}

func TestVectorClock_Compare_BeforeAfter_DifferentKeys(t *testing.T) {
	vc1 := core.NewVectorClock() // {A:1}
	vc2 := core.NewVectorClock() // {A:1, B:1}
	nodeA := core.NodeID("nodeA")
	nodeB := core.NodeID("nodeB")

	vc1[nodeA] = 1
	vc2[nodeA] = 1
	vc2[nodeB] = 1

	assert.Equal(t, core.Before, vc1.Compare(vc2))
	assert.Equal(t, core.After, vc2.Compare(vc1))
	assert.True(t, vc1.IsCausallyBefore(vc2))
	assert.False(t, vc2.IsCausallyBefore(vc1))
	assert.False(t, vc1.IsConcurrent(vc2))
}

func TestVectorClock_Compare_Concurrent(t *testing.T) {
	vc1 := core.NewVectorClock() // {A:1, B:2}
	vc2 := core.NewVectorClock() // {A:2, B:1}
	nodeA := core.NodeID("nodeA")
	nodeB := core.NodeID("nodeB")

	vc1[nodeA] = 1
	vc1[nodeB] = 2
	vc2[nodeA] = 2
	vc2[nodeB] = 1

	assert.Equal(t, core.Concurrent, vc1.Compare(vc2))
	assert.Equal(t, core.Concurrent, vc2.Compare(vc1))
	assert.False(t, vc1.IsCausallyBefore(vc2))
	assert.False(t, vc2.IsCausallyBefore(vc1))
	assert.True(t, vc1.IsConcurrent(vc2))
}

func TestVectorClock_Compare_Concurrent_DifferentKeys(t *testing.T) {
	vc1 := core.NewVectorClock() // {A:1}
	vc2 := core.NewVectorClock() // {B:1}
	nodeA := core.NodeID("nodeA")
	nodeB := core.NodeID("nodeB")

	vc1[nodeA] = 1
	vc2[nodeB] = 1

	assert.Equal(t, core.Concurrent, vc1.Compare(vc2))
	assert.Equal(t, core.Concurrent, vc2.Compare(vc1))
	assert.False(t, vc1.IsCausallyBefore(vc2))
	assert.False(t, vc2.IsCausallyBefore(vc1))
	assert.True(t, vc1.IsConcurrent(vc2))
}

func TestVectorClock_Copy(t *testing.T) {
	vc1 := core.NewVectorClock()
	nodeA := core.NodeID("nodeA")
	vc1[nodeA] = 5

	vc2 := vc1.Copy()
	assert.Equal(t, vc1[nodeA], vc2[nodeA])
	assert.NotSame(t, vc1, vc2) // Ensure it's a different map instance

	// Modify copy, original should be unchanged
	vc2.Increment(nodeA)
	assert.Equal(t, uint64(5), vc1[nodeA])
	assert.Equal(t, uint64(6), vc2[nodeA])
}

func TestVectorClock_String(t *testing.T) {
	vc := core.NewVectorClock()
	nodeA := core.NodeID("nodeA0123456789")
	nodeB := core.NodeID("nodeBabcdefghij")
	nodeC := core.NodeID("nodeC")

	vc[nodeB] = 3
	vc[nodeA] = 1
	vc[nodeC] = 5

	// Expect sorted output with shortened IDs
	expected := "{nodeA0...6789:1, nodeBa...ghij:3, nodeC:5}"
	assert.Equal(t, expected, vc.String())

	vcEmpty := core.NewVectorClock()
	assert.Equal(t, "{}", vcEmpty.String())
}

// Test Global Clock functions (basic sanity check)
func TestGlobalVectorClock(t *testing.T) {
	// Reset global state for test isolation if possible (tricky with globals)
	// For now, just test basic functionality assuming clean state or sequential tests
	nodeID := core.NodeID("testNodeGlobal")
	core.SetLocalNodeID(nodeID)

	vc1 := core.IncrementGlobalClock()
	assert.Equal(t, uint64(1), vc1[nodeID])

	vc2 := core.GetGlobalClock()
	assert.Equal(t, uint64(1), vc2[nodeID])
	assert.NotSame(t, vc1, vc2) // Should be copies

	externalClock := core.NewVectorClock()
	externalNode := core.NodeID("externalNode")
	externalClock[externalNode] = 5
	externalClock[nodeID] = 0 // Older timestamp for local node

	vc3 := core.MergeIntoGlobalClock(externalClock)
	assert.Equal(t, uint64(2), vc3[nodeID])       // Merged (max=1) then incremented
	assert.Equal(t, uint64(5), vc3[externalNode]) // From external

	vc4 := core.GetGlobalClock()
	assert.Equal(t, uint64(2), vc4[nodeID])
	assert.Equal(t, uint64(5), vc4[externalNode])
}
