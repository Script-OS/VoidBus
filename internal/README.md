# Internal Package

This package contains internal utilities for VoidBus. These utilities are NOT exposed to external packages.

## Files

### id.go
ID generation utilities:
- `GenerateID()`: Generate random UUID v4
- `GenerateSessionID()`: Generate session UUID
- `GenerateClientID()`: Generate client UUID
- `GenerateChallenge()`: Generate challenge bytes for handshake

**Constraints**: No external dependencies beyond Go standard library.

### checksum.go
Checksum calculation:
- `CalculateChecksum(data []byte) uint32`: CRC32 checksum

**Constraints**: No external dependencies beyond Go standard library.

### crypto.go
Cryptographic utilities for handshake:
- `ChallengeVerifier`: Interface for challenge verification
- `SimpleChallengeHandler`: Simple challenge handler implementation

**Constraints**: No external dependencies beyond Go standard library.

## Usage Rules

1. Internal package is for VoidBus internal use only
2. All functions must have no external dependencies
3. Each utility should have a single, clear responsibility
4. New utilities must be documented in this README