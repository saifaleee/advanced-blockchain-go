package core

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"sort"
	"strings"
)

// NodeID represents a unique identifier for a node in the network.
type NodeID string

// VectorClock represents the logical time across different shards or nodes.
// Key: ShardID (uint64)
// Value: Logical timestamp/version number (uint64)
type VectorClock map[uint64]uint64

// NewVectorClock creates an empty vector clock.
func NewVectorClock() VectorClock {
	return make(VectorClock)
}

// Increment increments the clock for the given node ID.
func (vc VectorClock) Increment(nodeID uint64) {
	vc[nodeID]++
}

// Serialize converts the VectorClock into a byte slice using gob encoding.
// Ensures deterministic output by sorting keys.
func (vc VectorClock) Serialize() ([]byte, error) {
	if vc == nil {
		return nil, fmt.Errorf("cannot serialize nil vector clock")
	}

	// Ensure deterministic serialization by sorting keys
	keys := make([]uint64, 0, len(vc))
	for k := range vc {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	err := encoder.Encode(vc)
	if err != nil {
		return nil, fmt.Errorf("failed to gob encode vector clock: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeVectorClock converts a byte slice back into a VectorClock using gob encoding.
func DeserializeVectorClock(data []byte) (VectorClock, error) {
	if data == nil {
		return nil, fmt.Errorf("cannot deserialize nil data")
	}

	var vc VectorClock
	buf := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buf)
	err := decoder.Decode(&vc)
	if err != nil {
		return nil, fmt.Errorf("failed to gob decode vector clock: %w", err)
	}
	return vc, nil
}

// Copy creates a deep copy of the VectorClock.
func (vc VectorClock) Copy() VectorClock {
	if vc == nil {
		return make(VectorClock) // Return new empty map if source is nil
	}
	newVc := make(VectorClock, len(vc))
	for k, v := range vc {
		newVc[k] = v
	}
	return newVc
}

// Merge combines another vector clock into this one, taking the maximum for each entry.
// Modifies the receiver clock (vc). Ensures vc is not nil.
func (vc VectorClock) Merge(other VectorClock) {
	if vc == nil || other == nil {
		log.Printf("[Warn] Attempted to merge nil vector clock(s). Skipping.")
		return
	}
	for k, vOther := range other {
		if vCurrent, ok := vc[k]; !ok || vOther > vCurrent {
			vc[k] = vOther
		}
	}
}

// Compare checks the causal relationship between two vector clocks.
// Returns:
//
//	-1 if vc < other (other happened after vc)
//	 0 if vc == other (identical)
//	 1 if vc > other (vc happened after other)
//	 2 if concurrent (neither happened strictly after the other, and not identical)
func (vc VectorClock) Compare(other VectorClock) int {
	// Handle nil cases consistently: nil is concurrent with non-nil, equal to nil.
	if vc == nil && other == nil {
		return 0 // Equal
	}
	if vc == nil {
		// Treat nil as the "empty" clock {}. An empty clock is Before any non-empty clock.
		if len(other) == 0 {
			return 0 // nil == empty
		} else {
			return -1 // nil < non-empty
		}
	}
	if other == nil {
		if len(vc) == 0 {
			return 0 // empty == nil
		} else {
			return 1 // non-empty > nil
		}
	}

	vcLessEqOther := true // Assume vc <= other
	otherLessEqVc := true // Assume other <= vc

	// Check vc <= other
	for k, vVc := range vc {
		vOther, ok := other[k]
		if !ok || vVc > vOther { // If k exists in vc but not other, OR vc[k] > other[k]
			vcLessEqOther = false
			break
		}
	}

	// Check other <= vc
	for k, vOther := range other {
		vVc, ok := vc[k]
		if !ok || vOther > vVc { // If k exists in other but not vc, OR other[k] > vc[k]
			otherLessEqVc = false
			break
		}
	}

	// Determine relationship
	if vcLessEqOther && otherLessEqVc {
		// If both <= hold, they must be equal. Check lengths for extra safety.
		if len(vc) == len(other) {
			return 0 // Equal
		} else {
			// This case implies one is a subset of the other with equal values where keys overlap.
			// e.g., {1:1} vs {1:1, 2:1}. Here {1:1} <= {1:1, 2:1} but not vice-versa.
			// The logic below should handle this via the strict inequality check.
			// If we reach here, it means vc <= other AND other <= vc, which implies Equal by definition.
			return 0 // Equal
		}
	} else if vcLessEqOther {
		// vc <= other, but not equal. Check if there's *strict* inequality.
		strictlyLess := false
		for k, vOther := range other {
			vVc, ok := vc[k]
			if !ok || vVc < vOther { // If k exists in other but not vc, OR vc[k] < other[k]
				strictlyLess = true
				break
			}
		}
		if strictlyLess {
			return -1 // Before (vc < other)
		} else {
			// vc <= other but not strictly less. This implies Equal, but the first check failed.
			// This path might indicate an issue or edge case, treat as Concurrent for safety?
			// Or re-evaluate: If vc <= other and not other <= vc, it MUST be Before.
			return -1 // Before
		}
	} else if otherLessEqVc {
		// other <= vc, but not equal. Check if there's *strict* inequality.
		strictlyLess := false
		for k, vVc := range vc {
			vOther, ok := other[k]
			if !ok || vOther < vVc { // If k exists in vc but not other, OR other[k] < vc[k]
				strictlyLess = true
				break
			}
		}
		if strictlyLess {
			return 1 // After (vc > other)
		} else {
			// other <= vc but not strictly less. Similar to above, implies Equal or an edge case.
			// If other <= vc and not vc <= other, it MUST be After.
			return 1 // After
		}
	} else {
		// Neither vc <= other nor other <= vc
		return 2 // Concurrent
	}
}

// String returns a sorted, string representation of the vector clock for logging.
func (vc VectorClock) String() string {
	if len(vc) == 0 {
		return "{}"
	}

	keys := make([]uint64, 0, len(vc))
	for k := range vc {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	var sb strings.Builder
	sb.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%d:%d", k, vc[k]))
	}
	sb.WriteString("}")
	return sb.String()
}
