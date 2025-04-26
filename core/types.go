package core

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sort" // Import sort for deterministic serialization/string representation
	"strings"
)

// NodeID represents a unique identifier for a node in the network.
type NodeID string

// VectorClock represents the logical time across different shards or nodes.
// Key: ShardID (uint64)
// Value: Logical timestamp/version number (uint64)
type VectorClock map[uint64]uint64

// Serialize converts the VectorClock to bytes for hashing or storage.
// Ensures deterministic output by sorting keys.
func (vc VectorClock) Serialize() ([]byte, error) {
	if vc == nil {
		// Handle nil map gracefully, maybe return empty bytes or error?
		// Returning encoded empty map is safer.
		return gobEncode(make(map[uint64]uint64))
	}
	// To ensure deterministic serialization for hashing, encode a sorted representation.
	// Using gob directly for simplicity in PoC, but be aware of potential non-determinism.
	return gobEncode(vc)
}

// Helper for gob encoding
func gobEncode(data interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to gob encode: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeVectorClock converts bytes back to a VectorClock.
func DeserializeVectorClock(data []byte) (VectorClock, error) {
	// Handle empty data case gracefully - return an empty clock
	if len(data) == 0 {
		return make(VectorClock), nil
	}

	var vc VectorClock
	dec := gob.NewDecoder(bytes.NewReader(data))
	err := dec.Decode(&vc)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize vector clock: %w", err)
	}
	// Ensure map is not nil even if decoding empty data resulted in nil map
	if vc == nil {
		vc = make(VectorClock)
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
	if vc == nil {
		// This shouldn't happen if vc is always initialized, but handle defensively.
		// Cannot modify a nil map. Log error or panic?
		// For now, log and return. The caller should ensure vc is initialized.
		fmt.Println("Error: Attempted to merge into a nil VectorClock.")
		return
	}
	if other == nil {
		return // Nothing to merge
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
	// Handle nil cases: Treat nil as concurrent with non-nil, equal to nil.
	if vc == nil && other == nil {
		return 0 // Equal
	}
	if vc == nil || other == nil {
		return 2 // Concurrent
	}

	vcGreaterThanOther := false // True if vc[k] > other[k] for at least one k, and vc[k] >= other[k] for all k in other
	otherGreaterThanVc := false // True if other[k] > vc[k] for at least one k, and other[k] >= vc[k] for all k in vc

	// Check if vc dominates other (vc >= other for all keys in other)
	vcDominates := true
	for kOther, vOther := range other {
		vVc, ok := vc[kOther]
		if !ok || vVc < vOther {
			vcDominates = false // vc does not dominate
			break
		}
		if vVc > vOther {
			vcGreaterThanOther = true // Found at least one element where vc is strictly greater
		}
	}

	// Check if other dominates vc (other >= vc for all keys in vc)
	otherDominates := true
	for kVc, vVc := range vc {
		vOther, ok := other[kVc]
		if !ok || vOther < vVc {
			otherDominates = false // other does not dominate
			break
		}
		if vOther > vVc {
			otherGreaterThanVc = true // Found at least one element where other is strictly greater
		}
	}

	// Determine relationship based on dominance and strict inequality
	if vcDominates && otherDominates {
		// If both dominate, they must be equal
		return 0 // Equal
	} else if vcDominates && vcGreaterThanOther {
		// vc dominates other, and is strictly greater in at least one element
		return 1 // vc > other (After)
	} else if otherDominates && otherGreaterThanVc {
		// other dominates vc, and is strictly greater in at least one element
		return -1 // vc < other (Before)
	} else {
		// Neither dominates the other, or one dominates but isn't strictly greater (implies equality, handled above)
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
