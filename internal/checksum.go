// Package internal provides checksum utilities.
package internal

import (
	"hash/crc32"
)

// CRC16Table is the CRC-16-CCITT lookup table.
var CRC16Table = [256]uint16{
	0x0000, 0x1021, 0x2042, 0x3063, 0x4084, 0x50A5, 0x60C6, 0x70E7,
	0x8108, 0x9129, 0xA14A, 0xB16B, 0xC18C, 0xD1AD, 0xE1CE, 0xF1EF,
	0x1231, 0x0210, 0x3273, 0x2252, 0x52B5, 0x4294, 0x72F7, 0x62D6,
	0x9339, 0x8318, 0xB37B, 0xA35A, 0xD3BD, 0xC39C, 0xF3FF, 0xE3DE,
	0x2462, 0x3443, 0x0420, 0x1401, 0x64E6, 0x74C7, 0x44A4, 0x5485,
	0xA56A, 0xB54B, 0x8528, 0x9509, 0xE5EE, 0xF5CF, 0xC5AC, 0xD58D,
	0x3653, 0x2672, 0x1611, 0x0630, 0x76D7, 0x66F6, 0x5695, 0x46B4,
	0xB75B, 0xA77A, 0x9719, 0x8738, 0xF7DF, 0xE7FE, 0xD79D, 0xC7BC,
	0x48C4, 0x58E5, 0x6886, 0x78A7, 0x0840, 0x1861, 0x2802, 0x3823,
	0xC9CC, 0xD9ED, 0xE98E, 0xF9AF, 0x8948, 0x9969, 0xA90A, 0xB92B,
	0x5AF5, 0x4AD4, 0x7AB7, 0x6A96, 0x1A71, 0x0A50, 0x3A33, 0x2A12,
	0xDBFD, 0xCBDC, 0xFBBF, 0xEB9E, 0x9B79, 0x8B58, 0xBB3B, 0xAB1A,
	0x6CA6, 0x7C87, 0x4CE4, 0x5CC5, 0x2C22, 0x3C03, 0x0C60, 0x1C41,
	0xEDAE, 0xFD8F, 0xCDEC, 0xDDCD, 0xAD2A, 0xBD0B, 0x8D68, 0x9D49,
	0x7E97, 0x6EB6, 0x5ED5, 0x4EF4, 0x3E13, 0x2E32, 0x1E51, 0x0E70,
	0xFF1F, 0xEF3E, 0xDF5D, 0xCF7C, 0xBF9B, 0xAFBA, 0x9FD9, 0x8FF8,
	0xF181, 0xE1A0, 0xD1C3, 0xC1E2, 0xB127, 0xA106, 0x9165, 0x8144,
	0x00E0, 0x10C1, 0x20A2, 0x3083, 0x4044, 0x5065, 0x6006, 0x7027,
	0x838C, 0x93AD, 0xA3CE, 0xB3EF, 0xC308, 0xD329, 0xE34A, 0xF36B,
	0x02B4, 0x1295, 0x22F6, 0x32D7, 0x423A, 0x521B, 0x627C, 0x725D,
}

// ComputeChecksumCRC16 calculates CRC-16-CCITT checksum.
func ComputeChecksumCRC16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc = (crc << 8) ^ CRC16Table[(crc>>8)^uint16(b)]
	}
	return crc
}

// VerifyChecksumCRC16 verifies data against expected CRC-16 checksum.
func VerifyChecksumCRC16(data []byte, expected uint16) bool {
	return ComputeChecksumCRC16(data) == expected
}

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

// ComputeChecksumCRC32 computes CRC32 checksum.
func ComputeChecksumCRC32(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// ComputeChecksumCRC32WithSalt computes CRC32 checksum with salt.
func ComputeChecksumCRC32WithSalt(data, salt []byte) uint32 {
	// Combine data and salt
	combined := make([]byte, len(data)+len(salt))
	copy(combined, data)
	copy(combined[len(data):], salt)
	return crc32.ChecksumIEEE(combined)
}
