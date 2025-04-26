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
func (vc NodeVectorClock) Compare(other NodeVectorClock) Causality {
	// Handle nil cases consistently by treating nil as an empty clock
	if vc == nil {
		vc = make(NodeVectorClock)
	}
	if other == nil {
		other = make(NodeVectorClock)
	}

	vcLessEqOther := true // Assume vc <= other initially
	otherLessEqVc := true // Assume other <= vc initially

	// Check vc <= other condition
	for k, vVc := range vc {
		vOther, ok := other[k]
		if !ok || vVc > vOther {
			vcLessEqOther = false
			break
		}
	}

	// Check other <= vc condition
	for k, vOther := range other {
		vVc, ok := vc[k]
		if !ok || vOther > vVc {
			otherLessEqVc = false
			break
		}
	}

	// Determine relationship
	if vcLessEqOther && otherLessEqVc {
		return Equal
	} else if vcLessEqOther {
		return Before // vc < other
	} else if otherLessEqVc {
		return After // other < vc (vc > other)
	} else {
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
