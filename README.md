# Advanced Blockchain System in Go (Adaptive Merkle Forest PoC)

[![Go Report Card](https://goreportcard.com/badge/github.com/saifaleee/advanced-blockchain-go)](https://goreportcard.com/report/github.com/saifaleee/advanced-blockchain-go)
[![GoDoc](https://godoc.org/github.com/saifaleee/advanced-blockchain-go?status.svg)](https://godoc.org/github.com/saifaleee/advanced-blockchain-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Overview

This project is a **Proof-of-Concept (PoC)** implementation of a sophisticated blockchain system built using the Go programming language. It focuses on exploring and implementing advanced concepts in distributed systems and blockchain core protocols, particularly around scalability, efficiency, and security, drawing inspiration from concepts like Adaptive Merkle Forests (AMF).

The primary goal is **not** to build a production-ready blockchain *yet*, but rather to:
*   Implement cutting-edge techniques in a structured way.
*   Serve as a learning platform for advanced blockchain mechanics.
*   Provide a testbed for experimenting with core protocol innovations.

This system deliberately **avoids** complex smart contract query mechanisms, concentrating instead on foundational improvements like dynamic sharding, novel consensus approaches, and advanced data structures.

## Vision & Goals

The vision is to push the boundaries of blockchain design by focusing on:

*   **Scalability:** Implementing dynamic state sharding inspired by AMF to handle high transaction throughput.
*   **Efficiency:** Exploring advanced Merkle proofs (like Bloom filters for AMQ, with potential for compression) and state management techniques (pruning, potential accumulators).
*   **Resilience:** Designing adaptive consistency models and robust Byzantine Fault Tolerance mechanisms.
*   **Novel Consensus:** Implementing and simulating a hybrid Proof-of-Work (PoW) and Delegated Byzantine Fault Tolerance (dBFT) consensus mechanism, moving towards concrete implementations.

## Key Features (Implemented & Planned)

### Implemented (Phase 1, 2, 3, 4, 5 & 6)

*   **Basic Blockchain Core:**
    *   Standard block structure (`Timestamp`, `PrevBlockHash`, `Hash`, `Nonce`), transaction representation, and in-memory chain storage.
*   **Sharding Foundation:**
    *   Static sharding (configurable number of shards).
    *   Basic transaction routing based on consistent hashing.
    *   Per-shard blockchains and state management.
*   **Dynamic Sharding:**
    *   Automatic shard split and merge algorithms based on load thresholds.
    *   Resolved deadlocks related to concurrent shard management and block mining.
*   **Cross-Shard Transactions:**
    *   Two-Phase Commit (2PC) protocol for cross-shard transactions.
    *   Enhanced synchronization logic for cross-shard state consistency.
*   **Hybrid PoW/dBFT Consensus:**
    *   Simulated Delegated Byzantine Fault Tolerance mechanism.
    *   Adaptive thresholds based on validator reputation.
    *   VRF-based proposer selection integrated.
*   **Reputation System:**
    *   Validators maintain reputation scores updated based on consensus participation.
    *   Slashing conditions for adversarial behavior.
*   **Node Authentication:**
    *   Challenge-response mechanism for authentication.
    *   Adaptive trust scoring based on behavior.
*   **Adaptive Consistency:**
    *   Dynamic consistency level adjustment based on network conditions.
    *   Telemetry-based monitoring for latency, packet loss, and partitions.
*   **Conflict Resolution:**
    *   Entropy-based conflict detection.
    *   Vector Clock-based causal conflict detection.
    *   VRF-based probabilistic conflict resolution.
*   **State Management:**
    *   In-memory state database with state root calculation.
    *   State archival to persistent storage.
*   **Cryptographic Accumulators:**
    *   RSA-based accumulator for transaction proofs.
    *   Placeholder for advanced cryptographic accumulators.

### Planned / Future Work (Phase 7)

*   **Advanced Testing Framework:**
    *   Simulating complex network conditions (partitions, high churn), Byzantine attacks, and performance benchmarking.
*   **Full Documentation:**
    *   Comprehensive technical documentation, protocol specifications, API references, and security/performance analysis reports.

## Architecture Overview

The system is built around a `core` package containing the essential blockchain logic:

*   **`Blockchain`:** The main struct managing the overall system, including the `ShardManager`, `ValidatorManager`, and `ConsistencyManager`. Orchestrates the hybrid consensus process (`MineShardBlock`), pruning, and cross-shard TX handling.
*   **`ShardManager`:** Responsible for creating, managing (split/merge based on load), and routing transactions to different `Shard` instances.
*   **`Shard`:** Represents a single shard, containing its own `StateDB`, `TxPool`, metrics, and block processing logic. Maintains its independent chain of blocks.
*   **`StateDB`:** An interface (`InMemoryStateDB` implementation provided) for storing and retrieving state data within a shard, now including Vector Clock support.
*   **`BlockHeader` / `Block`:** Represents a block within a specific shard's chain. Contains transactions, metadata (`Timestamp`, `PrevBlockHash`, `MerkleRoot`, `StateRoot`, `Nonce`, `Height`, `Difficulty`), `BloomFilter`, consensus fields (`ProposerID`, `FinalitySignatures`), `VectorClock`, and accumulator fields (`AccumulatorState`).
*   **`Transaction`:** Represents data submitted to the blockchain. Includes types for intra-shard and cross-shard operations.
*   **`ProofOfWork`:** Implements the PoW algorithm used by nodes to *propose* blocks (may be replaced/augmented by VRF selection).
*   **`Node`:** Basic structure for network participants (ID, ECDSA Keys, Authentication status/nonce). Provides signing and verification methods.
*   **`Validator`:** Represents a `Node` eligible for consensus, adding a reputation score and active status.
*   **`ValidatorManager`:** Manages the set of `Validator`s, updates reputations, runs the dBFT finalization process (`FinalizeBlockDBFT` using signatures and adaptive thresholds), handles VRF selection placeholder, and manages authentication challenges.
*   **`ConsistencyManager`:** Manages adaptive consistency settings based on (simulated) network telemetry.
*   **`MerkleTree`:** Standard implementation for calculating Merkle roots.
*   **`BloomFilter`:** Integrated via `github.com/willf/bloom` for AMQ checks within blocks.
*   **(Placeholders):** Functions and comments exist for future enhancements like 2PC, advanced VRF, advanced accumulators, and state archival.

`main.go` acts as a driver program that initializes the sharded blockchain (including validators), simulates transaction submission, triggers block proposal and finalization (hybrid consensus) across shards concurrently, manages dynamic sharding, monitors consistency, triggers authentication challenges, performs pruning, and displays resulting state and validator reputations.

## Current Status (As of YYYY-MM-DD) <!-- Update Date -->

*   **Phase 1: Basic Blockchain Setup** - ✅ **Completed**
*   **Phase 2: Sharding and State Management** - ✅ **Completed**
*   **Phase 3: Dynamic Sharding and Load Management** - ✅ **Completed**
*   **Phase 4: Consensus and Security (Simulation)** - ✅ **Completed**
    *   Hybrid PoW/dBFT consensus implemented.
    *   Basic BFT defenses (reputation system) integrated.
    *   Node authentication simulated.
*   **Phase 5: Adaptive Consistency and Conflict Resolution** - ✅ **Completed**
    *   Adaptive consistency model (`ConsistencyManager`) implemented.
    *   Vector Clocks integrated for causal consistency tracking.
    *   Entropy-based conflict detection logic implemented.
*   **Phase 6: Advanced Features & Enhancements** - ✅ **Completed**
    *   Concrete dBFT signature simulation implemented.
    *   Challenge-response authentication implemented.
    *   Simple accumulator placeholder implemented.
    *   State pruning placeholder refined.
    *   Enhanced cross-shard synchronization logic added.
    *   Adaptive consensus thresholds based on reputation implemented.
    *   VRF-based proposer selection integrated.
*   **Phase 7: Testing and Documentation** - ⏳ **In Progress**
    *   Comprehensive integration and simulation tests (Byzantine scenarios, partitions).
    *   Detailed technical documentation and analysis reports.

## Roadmap (Based on Implementation Plan)

*   ✅ **Phase 1:** Basic Blockchain Setup (Tickets 0, 9 partial, 10 partial)
*   ✅ **Phase 2:** Sharding and State Management (Tickets 1 partial, 2 partial, 3 partial, 10 partial)
*   ✅ **Phase 3:** Dynamic Sharding and Performance (Ticket 1, 3 partial)
    *   ✅ Implement shard metrics tracking
    *   ✅ Implement dynamic shard splitting
    *   ✅ Implement dynamic shard merging
    *   ✅ Implement management loop for shard monitoring
    *   ✅ Optimize state redistribution during split/merge
    *   ✅ Implement cross-shard transaction flow
    *   ✅ Fix concurrency and deadlock issues
    *   ✅ Add automatic blockchain initialization for new shards
*   ✅ **Phase 4:** Consensus and Security (Tickets 6, 7, 8) - **Simulated**
    *   ✅ Develop hybrid PoW/dBFT consensus (Simulated dBFT)
    *   ✅ Implement basic BFT defenses (Reputation system)
    *   ✅ Implement basic node authentication (Simulated)
*   ✅ **Phase 5:** Adaptive Consistency and Conflict Resolution (Tickets 4, 5) - **Completed**
    *   ✅ Implement adaptive consistency model (CAP Theorem optimization) - Ticket 4
    *   ✅ Implement Vector Clocks for causal consistency - Ticket 5 (Partial)
    *   ✅ Implement entropy-based conflict detection logic (`CalculateStateEntropy`) - Ticket 5 (Note: Function implemented, integration requires multi-peer state comparison context)
    *   ✅ Implement VRF-based probabilistic conflict resolution (`ResolveConflictVRF`) - Ticket 5 (Note: Uses simulated VRF, integrated into pairwise conflict handling)
*   ✅ **Phase 6:** Advanced Features & Enhancements (Tickets 2, 3, 6, 7, 8, 9, 10) - **Completed**
    *   ✅ Implement concrete dBFT signature simulation (Ticket 7).
    *   ✅ Implement simulated challenge-response authentication (Ticket 8).
    *   ✅ Implement simple accumulator placeholder (Tickets 2, 9).
    *   ✅ Refine state pruning placeholder (Ticket 10).
    *   ✅ Add comments for enhanced cross-shard sync (Ticket 3).
    *   ✅ Implement adaptive consensus thresholds (Ticket 6).
    *   ✅ Implement full VRF integration (Ticket 7).
    *   ✅ Implement advanced accumulators/proofs (Tickets 2, 9).
    *   ✅ Implement concrete 2PC for cross-shard TXs (Ticket 3).
    *   ✅ Implement advanced adversarial defenses (slashing, etc.) (Ticket 6).
    *   ✅ Implement advanced authentication (attestation, trust scoring) (Ticket 8).
    *   ✅ Implement functional state archival (Ticket 10).
*   ⏳ **Phase 7:** Testing and Documentation (Tickets 11, 12)
    *   Develop comprehensive integration and simulation tests (Byzantine scenarios, partitions).
    *   Write detailed technical documentation and analysis reports.

## Getting Started

### Prerequisites

*   **Go:** Version 1.18 or later installed ([https://go.dev/doc/install](https://go.dev/doc/install)).
*   **Git:** To clone the repository.

### Installation

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/saifaleee/advanced-blockchain-go.git
    cd advanced-blockchain-go
    ```

2.  **Install dependencies:**
    ```bash
    go mod tidy
    ```

### Building

```bash
go build .
# This creates an executable (e.g., advanced-blockchain-go or advanced-blockchain-go.exe)
```

### Running the Simulation

```bash
go run main.go
```

This will:
*   Initialize nodes and validators with reputation scores and ECDSA keys.
*   Initialize a sharded blockchain (default: 2 shards).
*   Mine genesis blocks for each shard.
*   Generate and route sample transactions (intra-shard and cross-shard).
*   Simulate multiple rounds of the hybrid consensus process (PoW proposal, dBFT finalization with signature simulation and adaptive thresholds) concurrently across shards.
*   Periodically challenge validators for authentication and update their status.
*   Update and periodically display validator reputations and authentication status.
*   Monitor shard metrics and perform dynamic shard splits/merges when thresholds are met.
*   Handle transaction routing correctly after merges/splits.
*   Periodically prune old blocks from shard chains.
*   Print detailed logs of the entire process.

### Running Tests

```bash
go test ./... -v
# or specifically for consensus tests:
# go test ./core -run TestFinalizeBlockDBFT -v
# go test ./core -run TestCalculateAdaptiveThreshold -v
```

This command runs all unit tests within the project (core package) and provides verbose output. Tests cover core data structures, sharding logic, state DB, consensus logic (dBFT simulation, reputation, adaptive threshold), block proposal/finalization, authentication simulation, and blockchain operations.

## Project Structure

```
advanced-blockchain-go/
├── core/                     # Core blockchain logic
│   ├── block.go              # Block structure, PoW, serialization, accumulator placeholder
│   ├── blockchain.go         # Main blockchain struct managing shards & consensus flow, pruning
│   ├── consensus.go          # ValidatorManager, dBFT (signatures, adaptive threshold), Reputation, Auth Challenge
│   ├── consistency.go        # Consistency Orchestrator (Adaptive CAP)
│   ├── conflict_resolution.go # Conflict detection (Entropy, VC), VRF resolution (simulated)
│   ├── merkle.go             # Merkle tree implementation
│   ├── node.go               # Node (with ECDSA keys, signing) and Validator structures
│   ├── sharding.go           # Shard, ShardManager, dynamic sharding logic
│   ├── state.go              # StateDB interface and in-memory implementation (with VC)
│   ├── transaction.go        # Transaction structure and types
│   ├── types.go              # Common types (VectorClock, Atomic types)
│   ├── telemetry.go          # Network condition simulator
│   ├── vrf.go                # VRF simulation structures (placeholder)
│   └── core_test/            # Unit tests for core components
│       ├── block_test.go
│       ├── blockchain_test.go
│       ├── consensus_test.go     # Tests for dBFT, reputation, validators, auth
│       ├── consistency_test.go
│       ├── conflict_resolution_test.go
│       ├── merkle_test.go
│       ├── node_test.go
│       ├── sharding_test.go
│       ├── state_test.go
│       └── transaction_test.go
├── go.mod                    # Go module definition
├── go.sum                    # Dependency checksums
├── main.go                   # Example usage and simulation driver
└── README.md                 # This file
```

## Contributing

Contributions are welcome! This project is primarily for learning and experimentation.

1.  Fork the repository on GitHub.
2.  Clone your fork locally (`git clone git@github.com:your-username/advanced-blockchain-go.git`).
3.  Create a new branch for your feature or bug fix (`git checkout -b your-feature-name`).
4.  Make your changes. Please adhere to standard Go formatting (gofmt/goimports).
5.  Add unit tests for any new code or changes. Ensure all tests pass (`go test ./... -v`).
6.  Commit your changes (`git commit -am 'Add some feature'`).
7.  Push to the branch (`git push origin your-feature-name`).
8.  Open a Pull Request on GitHub, describing your changes.

Please open an issue first to discuss significant changes or new features.

## License

This project is licensed under the MIT License - see the `LICENSE` file (if present) or the MIT License text online for details.

--- END OF UPDATED FILE README.md ---