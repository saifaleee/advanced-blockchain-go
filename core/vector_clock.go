package core

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// NodeVectorClock represents a vector clock mapping NodeIDs to logical timestamps.
type NodeVectorClock map[NodeID]uint64

// Causality represents the causal relationship between two vector clocks.
type Causality int

const (
	Equal      Causality = iota // Clocks represent the same causal history.
	Before                      // Clock A happened before Clock B.
	After                       // Clock A happened after Clock B.
	Concurrent                  // Clocks A and B are concurrent (no causal order).
)

// String returns a string representation of the Causality.
func (c Causality) String() string {
	switch c {
	case Equal:
		return "Equal"
	case Before:
		return "Before"
	case After:
		return "After"
	case Concurrent:
		return "Concurrent"
	default:
		return "Unknown"
	}
}

// NewNodeVectorClock creates an empty node vector clock.
func NewNodeVectorClock() NodeVectorClock {
	return make(NodeVectorClock)
}

// Increment increases the logical time for a specific node ID.
func (vc NodeVectorClock) Increment(nodeID NodeID) {
	vc[nodeID]++
}

// Merge combines two node vector clocks, taking the element-wise maximum.
// It modifies the receiver clock (vc).
func (vc NodeVectorClock) Merge(other NodeVectorClock) {
	for nodeID, timestamp := range other {
		if currentTimestamp, ok := vc[nodeID]; !ok || timestamp > currentTimestamp {
			vc[nodeID] = timestamp
		}
	}
}

// Compare determines the causal relationship between two node vector clocks (vc and other).
// Revised logic based on standard vector clock comparison rules.
func (vc NodeVectorClock) Compare(other NodeVectorClock) Causality {
	// Treat nil clocks as empty for comparison robustness
	if vc == nil {
		vc = make(NodeVectorClock)
	}
	if other == nil {
		other = make(NodeVectorClock)
	}

	vcLessEqOther := true      // vc <= other
	otherLessEqVc := true      // other <= vc
	vcStrictlyLess := false    // vc < other (at least one element is strictly less)
	otherStrictlyLess := false // other < vc (at least one element is strictly less)

	// Combine all unique node IDs from both clocks
	allNodeIDs := make(map[NodeID]struct{})
	for id := range vc {
		allNodeIDs[id] = struct{}{}
	}
	for id := range other {
		allNodeIDs[id] = struct{}{}
	}

	for id := range allNodeIDs {
		vcTs, vcOk := vc[id]
		otherTs, otherOk := other[id]

		// Check vc <= other condition
		if vcOk && (!otherOk || vcTs > otherTs) {
			vcLessEqOther = false
		}
		// Check other <= vc condition
		if otherOk && (!vcOk || otherTs > vcTs) {
			otherLessEqVc = false
		}

		// Check for strict inequality
		if vcOk && otherOk {
			if vcTs < otherTs {
				vcStrictlyLess = true
			}
			if otherTs < vcTs {
				otherStrictlyLess = true
			}
		} else if !vcOk && otherOk { // Key only in other, implies 0 < otherTs
			vcStrictlyLess = true
		} else if vcOk && !otherOk { // Key only in vc, implies vcTs > 0
			otherStrictlyLess = true
		}

		// Optimization: if neither can be less than or equal to the other, they are concurrent
		if !vcLessEqOther && !otherLessEqVc {
			return Concurrent
		}
	}

	// Final determination based on flags
	if vcLessEqOther && otherLessEqVc {
		return Equal // If vc <= other AND other <= vc, they must be equal
	} else if vcLessEqOther && vcStrictlyLess {
		return Before // vc <= other AND there is at least one element vc[k] < other[k]
	} else if otherLessEqVc && otherStrictlyLess {
		return After // other <= vc AND there is at least one element other[k] < vc[k]
	} else {
		// This handles cases like vc={1:1}, other={1:1, 2:1} where vc <= other but not strictly less.
		// Or vc={1:1, 2:1}, other={1:1} where other <= vc but not strictly less.
		// If one is a subset but not strictly smaller, the <= condition holds, but the strict flag doesn't.
		// If vc <= other but not strictly less, it means they are Equal or vc is Before (but not strictly).
		// If other <= vc but not strictly less, it means they are Equal or vc is After (but not strictly).
		// The Equal case is handled above. If we reach here, it means one direction holds (e.g., vc <= other)
		// but the strict condition didn't trigger (vcStrictlyLess is false).
		// This implies vc is Before other (but not strictly less in all dimensions where both exist).
		// Let's refine the return conditions based purely on <= flags:
		if vcLessEqOther { // We already know they are not Equal
			return Before
		}
		if otherLessEqVc { // We already know they are not Equal
			return After
		}

		// If neither <= holds, it must be Concurrent (this should have been caught by the optimization)
		return Concurrent
	}
}

// IsCausallyBefore checks if vc happened strictly before other.
func (vc NodeVectorClock) IsCausallyBefore(other NodeVectorClock) bool {
	return vc.Compare(other) == Before
}

// IsConcurrent checks if vc and other are concurrent.
func (vc NodeVectorClock) IsConcurrent(other NodeVectorClock) bool {
	return vc.Compare(other) == Concurrent
}

// Copy creates a deep copy of the node vector clock.
func (vc NodeVectorClock) Copy() NodeVectorClock {
	newVC := NewNodeVectorClock()
	for k, v := range vc {
		newVC[k] = v
	}
	return newVC
}

// String returns a sorted, string representation of the node vector clock for logging.
func (vc NodeVectorClock) String() string {
	if len(vc) == 0 {
		return "{}"
	}

	ids := make([]NodeID, 0, len(vc))
	for id := range vc {
		ids = append(ids, id)
	}
	// Sort by NodeID for consistent output
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	var sb strings.Builder
	sb.WriteString("{")
	for i, id := range ids {
		if i > 0 {
			sb.WriteString(", ")
		}
		// Shorten NodeID for readability
		shortID := id
		if len(id) > 8 {
			shortID = id[:4] + "..." + id[len(id)-4:]
		}
		sb.WriteString(fmt.Sprintf("%s:%d", shortID, vc[id]))
	}
	sb.WriteString("}")
	return sb.String()
}

// --- Global Vector Clock for the Node (Conceptual Example) ---
// In a real system, each node/process would maintain its own clock.
// This is a simplified global example for demonstration.

var (
	globalClockMutex sync.Mutex
	globalClock      = NewNodeVectorClock()
	localNodeID      NodeID // Assume this is set during node initialization
)

// SetLocalNodeID sets the ID for the current node (for global clock example).
func SetLocalNodeID(id NodeID) {
	localNodeID = id
}

// IncrementGlobalClock increments the global clock for the local node.
func IncrementGlobalClock() NodeVectorClock {
	globalClockMutex.Lock()
	defer globalClockMutex.Unlock()
	if localNodeID == "" {
		panic("Local Node ID not set for global vector clock")
	}
	globalClock.Increment(localNodeID)
	return globalClock.Copy() // Return a copy
}

// MergeIntoGlobalClock merges an external clock into the global clock.
func MergeIntoGlobalClock(externalClock NodeVectorClock) NodeVectorClock {
	globalClockMutex.Lock()
	defer globalClockMutex.Unlock()
	globalClock.Merge(externalClock)
	// Also increment local time after merge? Depends on model (often yes)
	if localNodeID != "" {
		globalClock.Increment(localNodeID)
	}
	return globalClock.Copy() // Return a copy
}

// GetGlobalClock returns a copy of the current global clock.
func GetGlobalClock() NodeVectorClock {
	globalClockMutex.Lock()
	defer globalClockMutex.Unlock()
	return globalClock.Copy()
}
