// Package negotiate provides bitmap utility functions.
package negotiate

// Clone creates a copy of channel bitmap.
func (b ChannelBitmap) Clone() ChannelBitmap {
	result := make([]byte, len(b))
	copy(result, b)
	return result
}

// Clone creates a copy of codec bitmap.
func (b CodecBitmap) Clone() CodecBitmap {
	result := make([]byte, len(b))
	copy(result, b)
	return result
}
