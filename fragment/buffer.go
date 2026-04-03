// Package fragment provides data fragmentation and reassembly for VoidBus v2.0.
package fragment

import (
	"sync"
	"time"

	"github.com/Script-OS/VoidBus/internal"
)

// SendBuffer represents the sender-side buffer for a session.
// Keeps original data and fragments until END_ACK received.
type SendBuffer struct {
	mu sync.RWMutex

	SessionID    string
	OriginalData []byte           // 原始数据（用于重传）
	EncodedData  []byte           // 编码后的数据
	Fragments    []*FragmentEntry // 分片信息
	CodecCodes   []string         // 选定的Codec代号组合
	CodecHash    [32]byte         // Codec链Hash
	DataHash     [32]byte         // 原始数据Hash（用于整体验证）
	SentTime     time.Time
	Retransmit   int  // 重传次数
	Complete     bool // 是否已收到END_ACK
	Expired      bool // 是否已超时
}

// FragmentEntry holds metadata for a single fragment (sender side).
type FragmentEntry struct {
	Index     uint16
	Data      []byte
	Checksum  uint32
	SentTime  time.Time
	Retried   int
	ChannelID string // 发送使用的Channel
}

// RecvBuffer represents the receiver-side buffer for a session.
type RecvBuffer struct {
	mu sync.RWMutex

	SessionID    string
	Total        uint16
	Received     map[uint16]*ReceivedFragment
	Missing      []uint16
	CodecDepth   uint8
	CodecHash    [32]byte
	DataHash     [32]byte // 期望的数据Hash
	StartTime    time.Time
	LastActivity time.Time
	Complete     bool
	Expired      bool
}

// ReceivedFragment holds a received fragment.
type ReceivedFragment struct {
	Index    uint16
	Data     []byte
	Checksum uint32
	RecvTime time.Time
	Verified bool
}

// NewSendBuffer creates a new SendBuffer.
func NewSendBuffer(sessionID string, originalData []byte) *SendBuffer {
	return &SendBuffer{
		SessionID:    sessionID,
		OriginalData: originalData,
		DataHash:     internal.ComputeDataHash(originalData),
		Fragments:    make([]*FragmentEntry, 0),
		SentTime:     time.Now(),
		Complete:     false,
	}
}

// SetCodecInfo sets codec chain information.
func (b *SendBuffer) SetCodecInfo(codes []string, hash [32]byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.CodecCodes = codes
	b.CodecHash = hash
}

// SetEncodedData sets the encoded data and creates fragments.
func (b *SendBuffer) SetEncodedData(encoded []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.EncodedData = encoded
}

// AddFragment adds a fragment info.
func (b *SendBuffer) AddFragment(index uint16, data []byte, checksum uint32, channelID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Fragments = append(b.Fragments, &FragmentEntry{
		Index:     index,
		Data:      data,
		Checksum:  checksum,
		SentTime:  time.Now(),
		ChannelID: channelID,
	})
}

// MarkComplete marks the buffer as complete (END_ACK received).
func (b *SendBuffer) MarkComplete() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Complete = true
}

// MarkExpired marks the buffer as expired.
func (b *SendBuffer) MarkExpired() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Expired = true
}

// IsComplete returns whether the buffer is complete.
func (b *SendBuffer) IsComplete() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Complete
}

// IsExpired returns whether the buffer is expired.
func (b *SendBuffer) IsExpired() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Expired
}

// GetRetransmitCount returns current retransmit count.
func (b *SendBuffer) GetRetransmitCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Retransmit
}

// IncrementRetransmit increments retransmit count.
func (b *SendBuffer) IncrementRetransmit() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Retransmit++
}

// GetMissingFragments returns fragments that need retransmission.
func (b *SendBuffer) GetMissingFragments(indices []uint16) []*FragmentEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]*FragmentEntry, 0, len(indices))
	for _, idx := range indices {
		for _, f := range b.Fragments {
			if f.Index == idx {
				result = append(result, f)
				break
			}
		}
	}
	return result
}

// GetDataHash returns the data hash.
func (b *SendBuffer) GetDataHash() [32]byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.DataHash
}

// GetCodecInfo returns codec codes and hash.
func (b *SendBuffer) GetCodecInfo() (codes []string, hash [32]byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.CodecCodes, b.CodecHash
}

// GetFragmentCount returns total fragment count.
func (b *SendBuffer) GetFragmentCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.Fragments)
}

// GetFragmentChannelIDs returns channel IDs used for sending fragments.
// Only available in debug mode for display purposes.
func (b *SendBuffer) GetFragmentChannelIDs() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	ids := make([]string, len(b.Fragments))
	for i, f := range b.Fragments {
		ids[i] = f.ChannelID
	}
	return ids
}

// NewRecvBuffer creates a new RecvBuffer.
func NewRecvBuffer(sessionID string, total uint16, codecDepth uint8, codecHash [32]byte, dataHash [32]byte) *RecvBuffer {
	missing := make([]uint16, total)
	for i := uint16(0); i < total; i++ {
		missing[i] = i
	}

	return &RecvBuffer{
		SessionID:    sessionID,
		Total:        total,
		Received:     make(map[uint16]*ReceivedFragment),
		Missing:      missing,
		CodecDepth:   codecDepth,
		CodecHash:    codecHash,
		DataHash:     dataHash,
		StartTime:    time.Now(),
		LastActivity: time.Now(),
		Complete:     false,
	}
}

// AddFragment adds a received fragment.
func (b *RecvBuffer) AddFragment(index uint16, data []byte, checksum uint32) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if already received
	if _, exists := b.Received[index]; exists {
		return false
	}

	// Verify checksum
	if !internal.VerifyChecksum(data, checksum) {
		return false
	}

	// Add to received
	b.Received[index] = &ReceivedFragment{
		Index:    index,
		Data:     data,
		Checksum: checksum,
		RecvTime: time.Now(),
		Verified: true,
	}

	// Update missing
	b.updateMissing()
	b.LastActivity = time.Now()

	// Check if complete
	if len(b.Missing) == 0 {
		b.Complete = true
	}

	return true
}

// updateMissing updates the missing list.
func (b *RecvBuffer) updateMissing() {
	b.Missing = make([]uint16, 0)
	for i := uint16(0); i < b.Total; i++ {
		if _, exists := b.Received[i]; !exists {
			b.Missing = append(b.Missing, i)
		}
	}
}

// GetMissing returns missing fragment indices.
func (b *RecvBuffer) GetMissing() []uint16 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Missing
}

// IsComplete returns whether all fragments are received.
func (b *RecvBuffer) IsComplete() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Complete
}

// Reassemble reassembles the data from received fragments.
func (b *RecvBuffer) Reassemble() ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.Complete {
		return nil, ErrFragmentIncomplete
	}

	// Sort and concatenate
	data := make([]byte, 0)
	for i := uint16(0); i < b.Total; i++ {
		fragment := b.Received[i]
		if fragment == nil {
			return nil, ErrFragmentMissing
		}
		data = append(data, fragment.Data...)
	}

	return data, nil
}

// VerifyDataHash verifies the reassembled data hash.
func (b *RecvBuffer) VerifyDataHash(data []byte) bool {
	return internal.VerifyDataHash(data, b.DataHash)
}

// MarkExpired marks the buffer as expired.
func (b *RecvBuffer) MarkExpired() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Expired = true
}

// IsExpired returns whether the buffer is expired.
func (b *RecvBuffer) IsExpired() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Expired
}

// GetProgress returns receive progress.
func (b *RecvBuffer) GetProgress() (received uint16, total uint16) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return uint16(len(b.Received)), b.Total
}

// GetLastActivity returns last activity time.
func (b *RecvBuffer) GetLastActivity() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.LastActivity
}

// GetCodecInfo returns codec depth and hash.
func (b *RecvBuffer) GetCodecInfo() (depth uint8, hash [32]byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.CodecDepth, b.CodecHash
}
