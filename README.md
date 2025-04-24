# Advanced Blockchain System in Go (Adaptive Merkle Forest PoC)

[![Go Report Card](https://goreportcard.com/badge/github.com/saifaleee/advanced-blockchain-go)](https://goreportcard.com/report/github.com/saifaleee/advanced-blockchain-go)
[![GoDoc](https://godoc.org/github.com/saifaleee/advanced-blockchain-go?status.svg)](https://godoc.org/github.com/saifaleee/advanced-blockchain-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Overview

This project is a **Proof-of-Concept (PoC)** implementation of a sophisticated blockchain system built using the Go programming language. It focuses on exploring and implementing advanced concepts in distributed systems and blockchain core protocols, particularly around scalability, efficiency, and security, drawing inspiration from concepts like Adaptive Merkle Forests (AMF).

The primary goal is **not** to build a production-ready blockchain *yet*, but rather to:
* Implement cutting-edge techniques in a structured way.
* Serve as a learning platform for advanced blockchain mechanics.
* Provide a testbed for experimenting with core protocol innovations.

This system deliberately **avoids** complex smart contract query mechanisms, concentrating instead on foundational improvements like dynamic sharding, novel consensus approaches, and advanced data structures.

## Vision & Goals

The vision is to push the boundaries of blockchain design by focusing on:

* **Scalability:** Implementing dynamic state sharding inspired by AMF to handle high transaction throughput.
* **Efficiency:** Exploring advanced Merkle proofs (like Bloom filters for AMQ, with potential for compression) and state management techniques (pruning, potential accumulators).
* **Resilience:** Designing adaptive consistency models and robust Byzantine Fault Tolerance mechanisms.
* **Novel Consensus:** Implementing a hybrid Proof-of-Work (PoW) and Delegated Byzantine Fault Tolerance (dBFT) consensus mechanism.

## Key Features (Implemented & Planned)

### Implemented (Phase 1, 2 & Partial Phase 3 Completed)

* **Basic Blockchain Core:** Standard block structure (`Timestamp`, `PrevBlockHash`, `Hash`, `Nonce`), transaction representation, and in-memory chain storage.
* **Sharding Foundation:**
    * Static sharding (configurable number of shards).
    * Basic transaction routing based on transaction ID hash modulo shard count.
    * Per-shard block chains (each shard maintains its own sequence of blocks).
    * Per-shard state management using an in-memory key-value store (`InMemoryStateDB`).
    * Per-shard transaction pool (`TxPool`).
* **Dynamic Sharding (Ticket 1):**
    * Implementation of shard metrics tracking (transaction count, state size).
    * Automatic shard split algorithm when transaction load exceeds configurable thresholds.
    * Automatic shard merge algorithm for underutilized shards.
    * Robust fallback routing mechanism when target shards don't exist due to merges.
    * Management loop for continuous monitoring and dynamic shard adjustments.
* **Merkle Trees:** Standard Merkle trees for aggregating transaction IDs within each block (`MerkleRoot`).
* **Approximate Membership Queries (AMQ):** Bloom Filters integrated into blocks (`block.BloomFilter`) to allow fast, probabilistic checking of transaction inclusion (part of Ticket 2).
* **Proof-of-Work (PoW):** Basic hash-based PoW consensus mechanism used for block mining within each shard.
* **Basic Cross-Shard Handling:** Structures (`CrossShardTxInit`, `CrossShardReceipt`) and placeholder logic for initiating and potentially processing transactions that span shards. Current implementation logs generation of receipts on the source shard during mining.
* **Basic State Pruning:** Placeholder function to demonstrate removing old blocks from the in-memory shard chains below a specified height.
* **Comprehensive Unit Tests:** Tests covering core data structures and functionalities (Blocks, Transactions, Merkle Trees, Blockchain, Sharding, State).

### Planned (Future Phases)

* **Advanced Merkle Proofs (Ticket 2):** Exploring probabilistic proof compression techniques and potentially cryptographic accumulators for more compact state proofs.
* **Cross-Shard State Synchronization (Ticket 3):** Developing a robust protocol for atomic or eventually consistent state changes across shards, likely involving verifiable receipts/proofs.
* **Adaptive Consistency Model (Ticket 4):** Dynamically adjusting consistency levels (e.g., strong vs. eventual) based on network conditions.
* **Advanced Conflict Resolution (Ticket 5):** Implementing mechanisms using vector clocks, entropy, or VRFs to detect and resolve state conflicts, especially in cross-shard scenarios.
* **Multi-Layer Adversarial Defense (Ticket 6):** Enhancing BFT with reputation systems, adaptive thresholds, and potentially ZKPs or MPC.
* **Hybrid Consensus Protocol (Ticket 7):** Fully implementing the hybrid PoW/dBFT model, potentially using VRFs for delegate selection.
* **Advanced Node Authentication (Ticket 8):** Implementing continuous cryptographic challenges and adaptive trust scoring.
* **Advanced Block Composition (Ticket 9):** Integrating cryptographic accumulators into block headers, potentially using multi-level Merkle trees for sharded state proofs.
* **State Compression and Archival (Ticket 10):** Implementing efficient state pruning while maintaining history via cryptographic commitments, potentially integrating with distributed storage for archival.
* **Advanced Testing Framework (Ticket 11):** Simulating network partitions, Byzantine faults, and complex sharding scenarios.
* **Full Documentation (Ticket 12):** Detailed technical documentation, protocol specifications, and analysis.

## Architecture Overview

The system is built around a `core` package containing the essential blockchain logic:

* **`Blockchain`:** The main struct managing the overall system, including the `ShardManager`.
* **`ShardManager`:** Responsible for creating, managing, and routing transactions to different `Shard` instances. Now includes functionality for dynamic shard splitting and merging based on load metrics.
* **`Shard`:** Represents a single shard, containing its own `StateDB`, `TxPool`, metrics, and processing logic. Each shard maintains its own independent chain of blocks.
* **`StateDB`:** An interface (`InMemoryStateDB` implementation provided) for storing and retrieving state data (e.g., account balances - although accounts aren't explicitly modeled yet) within a shard.
* **`Block`:** Represents a block within a specific shard's chain. Contains transactions, metadata, `MerkleRoot`, `BloomFilter`, and links to the previous block *in the same shard*.
* **`Transaction`:** Represents data submitted to the blockchain. Includes types for intra-shard and cross-shard operations.
* **`ProofOfWork`:** Implements the basic PoW algorithm used by shards to mine new blocks.
* **`MerkleTree`:** Standard implementation for calculating Merkle roots.
* **`BloomFilter`:** Integrated via `github.com/willf/bloom` for AMQ checks within blocks.

`main.go` acts as a driver program that initializes the sharded blockchain, simulates transaction submission, triggers block mining across shards concurrently, and displays the resulting state.

## Current Status (As of 2025-04-24)

* **Phase 1: Basic Blockchain Setup** - **Completed**
* **Phase 2: Sharding and State Management** - **Completed**
    * Foundation for sharding is implemented.
    * Bloom filters are integrated into blocks.
    * Basic structures for cross-shard transactions exist.
    * State management is per-shard (in-memory).
* **Phase 3: Dynamic Sharding and Load Management** - **Partially Completed**
    * Dynamic shard splitting based on transaction load implemented.
    * Dynamic shard merging for underutilized shards implemented.
    * Management loop for continuous monitoring of shards implemented.
    * Improved transaction routing with fallback mechanisms.
    * Metrics tracking for shards implemented.
* The project is currently a **Proof-of-Concept**. More advanced features like full cross-shard atomicity/consistency and the hybrid consensus mechanism are **not yet implemented**.

## Getting Started

### Prerequisites

* **Go:** Version 1.18 or later installed ([https://go.dev/doc/install](https://go.dev/doc/install)).
* **Git:** To clone the repository.

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
- Initialize a sharded blockchain (default: 4 shards).
- Mine genesis blocks for each shard.
- Generate and route sample transactions (including some cross-shard placeholders).
- Simulate multiple rounds of concurrent block mining across the shards.
- Print the detailed contents of each block in every shard's chain.
- Validate the integrity of all shard chains.
- Demonstrate the (placeholder) pruning mechanism.
- Show the final block count per shard after pruning.

Output will show logs detailing transaction routing, mining progress per shard (Nonce found, duration), cross-shard processing placeholders, block contents, validation results, and pruning actions.

### Running Tests

```bash
go test ./... -v
```

This command runs all unit tests within the project (core package) and provides verbose output. Tests cover core data structures, sharding logic, state DB, Bloom filters, and blockchain operations.

## Project Structure

```
advanced-blockchain-go/
├── core/                     # Core blockchain logic
│   ├── block.go              # Block structure, PoW, serialization
│   ├── blockchain.go         # Main blockchain struct managing shards
│   ├── merkle.go             # Merkle tree implementation
│   ├── sharding.go           # Shard, ShardManager, routing logic
│   ├── state.go              # StateDB interface and in-memory implementation
│   ├── transaction.go        # Transaction structure and types
│   └── core_test/            # Unit tests for core components
│       ├── block_test.go
│       ├── blockchain_test.go
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

- ✅ **Phase 1:** Basic Blockchain Setup (Tickets 0, 9 partial, 10 partial)
- ✅ **Phase 2:** Sharding and State Management (Tickets 1 partial, 2 partial, 3 partial, 10 partial)
- 🟡 **Phase 3:** Dynamic Sharding and Performance (Ticket 1 mostly complete)
  - ✅ Implement shard metrics tracking
  - ✅ Implement dynamic shard splitting
  - ✅ Implement dynamic shard merging
  - ✅ Implement management loop for shard monitoring
  - ◻️ Optimize state redistribution during split/merge
- ◻️ **Phase 4:** Consensus and Security (Tickets 6, 7, 8)
  - Develop hybrid PoW/dBFT consensus.
  - Implement multi-layer BFT defenses (reputation, etc.).
  - Implement advanced node authentication.
- ◻️ **Phase 5:** Adaptive Consistency and Conflict Resolution (Tickets 4, 5)
  - Implement adaptive consistency model (CAP Theorem optimization).
  - Implement advanced conflict detection and resolution.
- ◻️ **Phase 6:** Testing and Documentation (Tickets 11, 12)
  - Develop comprehensive integration and simulation tests.
  - Write detailed technical documentation and analysis reports.
  - Complete implementation of all features from earlier tickets (e.g., full cross-shard sync).

## Contributing

Contributions are welcome! This project is primarily for learning and experimentation.

1. Fork the repository on GitHub.
2. Clone your fork locally (`git clone git@github.com:saifaleee/advanced-blockchain-go.git`).
3. Create a new branch for your feature or bug fix (`git checkout -b your-feature-name`).
4. Make your changes. Please adhere to standard Go formatting (gofmt/goimports).
5. Add unit tests for any new code or changes. Ensure all tests pass (`go test ./... -v`).
6. Commit your changes (`git commit -am 'Add some feature'`).
7. Push to the branch (`git push origin your-feature-name`).
8. Open a Pull Request on GitHub, describing your changes.

Please open an issue first to discuss significant changes or new features.

## License

This project is licensed under the MIT License - see the LICENSE file for details.