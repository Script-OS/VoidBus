// Package internal provides checksum utilities.
package internal

import (
	"hash/crc32"
)

// CalculateChecksum calculates CRC32 checksum of data.
func CalculateChecksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// VerifyChecksum verifies data against expected checksum.
func VerifyChecksum(data []byte, expected uint32) bool {
	return CalculateChecksum(data) == expected
}

// CalculateChecksumBytes returns checksum as 4-byte slice.
func CalculateChecksumBytes(data []byte) []byte {
	checksum := CalculateChecksum(data)
	result := make([]byte, 4)
	result[0] = byte(checksum >> 24)
	result[1] = byte(checksum >> 16)
	result[2] = byte(checksum >> 8)
	result[3] = byte(checksum)
	return result
}

// BytesToChecksum converts 4-byte slice to checksum.
func BytesToChecksum(data []byte) uint32 {
	if len(data) < 4 {
		return 0
	}
	return uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
}

// CalculateChecksum returns CRC32 checksum as uint32.
// Alias for CalculateChecksum for clarity.
func CalculateChecksumUint32(data []byte) uint32 {
	return CalculateChecksum(data)
}
