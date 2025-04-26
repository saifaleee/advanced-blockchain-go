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
*   **Novel Consensus:** Implementing and simulating a hybrid Proof-of-Work (PoW) and Delegated Byzantine Fault Tolerance (dBFT) consensus mechanism.

## Key Features (Implemented & Planned)

### Implemented (Phase 1, 2, 3 & 4)

*   **Basic Blockchain Core:** Standard block structure (`Timestamp`, `PrevBlockHash`, `Hash`, `Nonce`), transaction representation, and in-memory chain storage.
*   **Sharding Foundation:**
    *   Static sharding (configurable number of shards).
    *   Basic transaction routing based on consistent hashing.
    *   Per-shard block chains (each shard maintains its own sequence of blocks).
    *   Per-shard state management using an in-memory key-value store (`InMemoryStateDB`).
    *   Per-shard transaction pool (`TxPool`).
*   **Dynamic Sharding (Ticket 1 - Phase 3):**
    *   Implementation of shard metrics tracking (transaction count, state size, block count).
    *   Automatic shard split algorithm when load thresholds are exceeded.
    *   Automatic shard merge algorithm for underutilized shards.
    *   Robust fallback routing mechanism for transactions targeting non-existent shards (e.g., after merge).
    *   Management loop for continuous monitoring and dynamic shard adjustments.
    *   Efficient state migration during shard splits with load balancing.
    *   Automatic (lazy) blockchain initialization for newly created shards during the first mining attempt.
    *   Resolved deadlocks related to concurrent shard management and block mining.
*   **Cross-Shard Transactions (Ticket 3 partial - Phase 3):**
    *   Implemented and tested cross-shard transaction flow (Initiate/Finalize).
    *   Transaction initiation in source shard with routing of finalization TX to destination shard.
    *   Finalization transaction processing in destination shard.
*   **Merkle Trees:** Standard Merkle trees for aggregating transaction IDs within each block (`MerkleRoot`).
*   **Approximate Membership Queries (AMQ):** Bloom Filters integrated into blocks (`block.BloomFilter`) to allow fast, probabilistic checking of transaction inclusion (part of Ticket 2).
*   **Hybrid PoW/dBFT Consensus (Simulated - Phase 4):**
    *   **PoW for Proposal:** Blocks are proposed by nodes after solving a PoW puzzle (`ProofOfWork`, `ProposeBlock`). Block headers include `ProposerID`.
    *   **dBFT for Finalization:** A simulated Delegated Byzantine Fault Tolerance mechanism finalizes proposed blocks.
        *   Requires > 2/3 agreement from eligible validators (`SimulateDBFT`).
        *   Finalized blocks store agreeing validator IDs (`FinalitySignatures` - simulated).
*   **Basic Reputation System (Phase 4):**
    *   Validators maintain reputation scores (`Validator.Reputation`).
    *   Scores are updated based on consensus participation (rewards for success, penalties for failure) handled by `ValidatorManager`.
*   **Simulated Node Authentication (Phase 4):**
    *   Nodes have an authentication status (`Node.IsAuthenticated`).
    *   Consensus eligibility checks consider this status (`GetActiveValidatorsForShard`).
*   **Node/Validator Structures (Phase 4):**
    *   `Node`: Represents network participants with ID, PublicKey (placeholder), authentication status.
    *   `Validator`: Extends `Node` with reputation score and active status.
    *   `ValidatorManager`: Manages validators, reputation, and the dBFT simulation.
*   **Basic State Pruning:** Placeholder function (`PruneChain`) to demonstrate removing old blocks.
*   **Comprehensive Unit Tests:** Tests covering core data structures and functionalities (Blocks, Transactions, Merkle Trees, Blockchain, Sharding, State, **Consensus**).

### Planned (Future Phases)

*   **Advanced Merkle Proofs (Ticket 2):** Exploring probabilistic proof compression techniques and potentially cryptographic accumulators for more compact state proofs.
*   **Cross-Shard State Synchronization Improvements (Ticket 3):** Enhancing the existing implementation with stronger atomicity guarantees (e.g., two-phase commit variations) and more efficient verification (e.g., light clients, state proofs).
*   **Adaptive Consistency Model (Ticket 4):** Dynamically adjusting consistency levels (e.g., strong vs. eventual) based on network conditions or transaction types.
*   **Advanced Conflict Resolution (Ticket 5):** Implementing mechanisms using vector clocks, entropy, or VRFs to detect and resolve state conflicts, especially in cross-shard scenarios or under partitions.
*   **Multi-Layer Adversarial Defense (Ticket 6):** Enhancing BFT with adaptive thresholds based on reputation/stake, potentially integrating ZKPs or MPC for privacy/validation, slashing conditions.
*   **Hybrid Consensus Protocol Enhancements (Ticket 7):** Replace simulated dBFT with a concrete implementation (e.g., using cryptographic signatures), potentially using VRFs for dynamic delegate/proposer selection instead of fixed validators/PoW race.
*   **Advanced Node Authentication (Ticket 8):** Implementing real cryptographic challenges (e.g., sign-verify), potentially continuous attestation, and adaptive trust scoring beyond simple boolean authentication.
*   **Advanced Block Composition (Ticket 9):** Integrating cryptographic accumulators (e.g., RSA) into block headers, potentially using multi-level Merkle trees for efficient sharded state proofs.
*   **State Compression and Archival (Ticket 10):** Implementing efficient state pruning while maintaining history via cryptographic commitments (e.g., Merkle proofs of pruned state), potentially integrating with distributed storage (IPFS) for archival.
*   **Advanced Testing Framework (Ticket 11):** Simulating network partitions, Byzantine faults (double spending, equivocation), complex sharding scenarios (rapid split/merge), and performance benchmarking.
*   **Full Documentation (Ticket 12):** Detailed technical documentation, protocol specifications, API references, and analysis reports.

## Architecture Overview

The system is built around a `core` package containing the essential blockchain logic:

*   **`Blockchain`:** The main struct managing the overall system, including the `ShardManager` and now the `ValidatorManager`. Orchestrates the hybrid consensus process (`MineShardBlock`).
*   **`ShardManager`:** Responsible for creating, managing (split/merge based on load), and routing transactions to different `Shard` instances.
*   **`Shard`:** Represents a single shard, containing its own `StateDB`, `TxPool`, metrics, and block processing logic. Maintains its independent chain of blocks.
*   **`StateDB`:** An interface (`InMemoryStateDB` implementation provided) for storing and retrieving state data within a shard.
*   **`Block`:** Represents a block within a specific shard's chain. Contains transactions, metadata (`Timestamp`, `PrevBlockHash`, `MerkleRoot`, `StateRoot`, `Nonce`, `Height`, `Difficulty`), `BloomFilter`, and new consensus fields: `ProposerID`, `FinalitySignatures` (simulated).
*   **`Transaction`:** Represents data submitted to the blockchain. Includes types for intra-shard and cross-shard operations.
*   **`ProofOfWork`:** Implements the PoW algorithm used by nodes to *propose* blocks.
*   **`Node`:** Basic structure for network participants (ID, PublicKey placeholder, Authentication status).
*   **`Validator`:** Represents a `Node` eligible for consensus, adding a reputation score and active status.
*   **`ValidatorManager`:** Manages the set of `Validator`s, updates reputations, and runs the `SimulateDBFT` process for block finalization.
*   **`MerkleTree`:** Standard implementation for calculating Merkle roots.
*   **`BloomFilter`:** Integrated via `github.com/willf/bloom` for AMQ checks within blocks.

`main.go` acts as a driver program that initializes the sharded blockchain (including validators), simulates transaction submission, triggers block proposal and finalization (hybrid consensus) across shards concurrently, manages dynamic sharding, and displays resulting state and validator reputations.

## Current Status (As of 2025-04-24)

*   **Phase 1: Basic Blockchain Setup** - ✅ **Completed**
*   **Phase 2: Sharding and State Management** - ✅ **Completed**
*   **Phase 3: Dynamic Sharding and Load Management** - ✅ **Completed**
    *   Dynamic shard splitting and merging are functional.
    *   Cross-shard transaction routing and processing implemented.
    *   Concurrency issues (deadlocks) resolved.
    *   Lazy initialization of new shard chains implemented.
*   **Phase 4: Consensus and Security (Simulation)** - ✅ **Completed (Simulated)**
    *   Hybrid PoW proposal / simulated dBFT finalization implemented.
    *   Basic validator reputation system integrated with consensus.
    *   Simulated node authentication check added to consensus eligibility.
*   The project demonstrates working dynamic sharding, cross-shard transactions, and a simulated hybrid consensus mechanism with reputation. It serves as a solid foundation for exploring more advanced concepts outlined in future phases.

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
*   Initialize nodes and validators with reputation scores.
*   Initialize a sharded blockchain (default: 2 shards).
*   Mine genesis blocks for each shard.
*   Generate and route sample transactions (intra-shard and cross-shard).
*   Simulate multiple rounds of the hybrid consensus process (PoW proposal, dBFT finalization) concurrently across shards.
*   Update and periodically display validator reputations.
*   Monitor shard metrics and perform dynamic shard splits/merges when thresholds are met.
*   Handle transaction routing correctly after merges/splits.
*   Print detailed logs of the entire process.

### Running Tests

```bash
go test ./... -v
# or specifically for consensus tests:
# go test ./core -run TestSimulateDBFT -v
# go test ./core -run TestBlockchainConsensus -v
```

This command runs all unit tests within the project (core package) and provides verbose output. Tests cover core data structures, sharding logic, state DB, consensus logic (dBFT simulation, reputation), block proposal/finalization, and blockchain operations.

## Project Structure

```
advanced-blockchain-go/
├── core/                     # Core blockchain logic
│   ├── block.go              # Block structure, PoW, serialization
│   ├── blockchain.go         # Main blockchain struct managing shards & consensus flow
│   ├── consensus.go          # ValidatorManager, dBFT simulation, Reputation logic
│   ├── merkle.go             # Merkle tree implementation
│   ├── node.go               # Node and Validator structures
│   ├── sharding.go           # Shard, ShardManager, dynamic sharding logic
│   ├── state.go              # StateDB interface and in-memory implementation
│   ├── transaction.go        # Transaction structure and types
│   └── core_test/            # Unit tests for core components
│       ├── block_test.go
│       ├── blockchain_test.go
│       ├── consensus_test.go     # Tests for dBFT, reputation, validators
│       ├── merkle_test.go
│       ├── sharding_test.go
│       ├── state_test.go
│       └── transaction_test.go
├── go.mod                    # Go module definition
├── go.sum                    # Dependency checksums
├── main.go                   # Example usage and simulation driver
└── README.md                 # This file
```

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
*   ◻️ **Phase 6:** Advanced Features & Enhancements (Tickets 2, 3, 6, 7, 8, 9, 10)
    *   Implement concrete dBFT (signatures, VRFs?).
    *   Implement advanced authentication/attestation.
    *   Implement advanced Merkle proofs/accumulators.
    *   Implement state pruning/archival.
    *   Enhance cross-shard sync guarantees.
*   ◻️ **Phase 7:** Testing and Documentation (Tickets 11, 12)
    *   Develop comprehensive integration and simulation tests (Byzantine scenarios, partitions).
    *   Write detailed technical documentation and analysis reports.

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