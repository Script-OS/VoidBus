// Package simple provides a simple Fragment implementation.
//
// SimpleFragment implements a header-based fragmentation scheme:
// - Header: 24 bytes (ID[16] + Index[2] + Total[2] + Size[4])
// - Data: variable length
//
// Design Constraints (see docs/ARCHITECTURE.md §2.1.5):
// - FragmentInfo.ID CAN be exposed (random UUID)
// - FragmentInfo.Index/Total CAN be exposed
package simple

import (
	"encoding/binary"

	"VoidBus/fragment"
	"VoidBus/internal"
)

const (
	// HeaderSize is the fragment header size in bytes
	HeaderSize = 24
	// IDSize is the ID field size
	IDSize = 16
	// MinFragmentSize is the minimum allowed fragment size
	MinFragmentSize = 64
)

// SimpleFragment implements fragment.Fragment interface.
type SimpleFragment struct {
	config fragment.FragmentConfig
}

// New creates a new SimpleFragment instance.
func New(config fragment.FragmentConfig) *SimpleFragment {
	return &SimpleFragment{config: config}
}

// Split splits data into fragments with headers.
func (f *SimpleFragment) Split(data []byte, maxSize int) ([][]byte, error) {
	if maxSize < MinFragmentSize {
		return nil, fragment.ErrInvalidFragmentSize
	}
	if len(data) == 0 {
		return nil, nil
	}

	dataCapacity := maxSize - HeaderSize
	if dataCapacity <= 0 {
		return nil, fragment.ErrInvalidFragmentSize
	}

	totalFragments := (len(data) + dataCapacity - 1) / dataCapacity
	if totalFragments > 65535 {
		return nil, fragment.ErrFragmentFailed
	}

	groupID := internal.GenerateID()
	fragments := make([][]byte, 0, totalFragments)
	offset := 0

	for i := 0; i < totalFragments; i++ {
		end := offset + dataCapacity
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]
		offset = end

		info := fragment.FragmentInfo{
			ID:     groupID,
			Index:  uint16(i),
			Total:  uint16(totalFragments),
			Size:   uint32(len(chunk)),
			IsLast: i == totalFragments-1,
		}

		frag, err := f.SetFragmentInfo(chunk, info)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, frag)
	}

	return fragments, nil
}

// Reassemble reassembles fragments into original data.
func (f *SimpleFragment) Reassemble(frags [][]byte) ([]byte, error) {
	if len(frags) == 0 {
		return nil, nil
	}

	firstInfo, err := f.GetFragmentInfo(frags[0])
	if err != nil {
		return nil, err
	}

	if len(frags) != int(firstInfo.Total) {
		return nil, fragment.ErrFragmentIncomplete
	}

	result := make([]byte, 0, int(firstInfo.Total)*f.config.MaxFragmentSize)
	for i, frag := range frags {
		info, err := f.GetFragmentInfo(frag)
		if err != nil {
			return nil, err
		}
		if info.ID != firstInfo.ID {
			return nil, fragment.ErrFragmentMismatch
		}
		if int(info.Index) != i {
			return nil, fragment.ErrFragmentMismatch
		}
		if len(frag) < HeaderSize {
			return nil, fragment.ErrFragmentCorrupted
		}
		result = append(result, frag[HeaderSize:]...)
	}

	return result, nil
}

// GetFragmentInfo extracts fragment metadata from fragment data.
func (f *SimpleFragment) GetFragmentInfo(fragmentData []byte) (fragment.FragmentInfo, error) {
	if len(fragmentData) < HeaderSize {
		return fragment.FragmentInfo{}, fragment.ErrFragmentCorrupted
	}

	info := fragment.FragmentInfo{}
	idBytes := make([]byte, IDSize)
	copy(idBytes, fragmentData[0:IDSize])
	info.ID = string(idBytes)
	info.Index = binary.BigEndian.Uint16(fragmentData[IDSize : IDSize+2])
	info.Total = binary.BigEndian.Uint16(fragmentData[IDSize+2 : IDSize+4])
	info.Size = binary.BigEndian.Uint32(fragmentData[IDSize+4 : IDSize+8])
	info.IsLast = info.Index == info.Total-1

	if f.config.EnableChecksum {
		info.Checksum = internal.CalculateChecksum(fragmentData[HeaderSize:])
	}

	return info, nil
}

// SetFragmentInfo adds fragment metadata header to data.
func (f *SimpleFragment) SetFragmentInfo(data []byte, info fragment.FragmentInfo) ([]byte, error) {
	header := make([]byte, HeaderSize)
	idBytes := []byte(info.ID)
	if len(idBytes) > IDSize {
		idBytes = idBytes[:IDSize]
	}
	copy(header[0:], idBytes)
	binary.BigEndian.PutUint16(header[IDSize:], info.Index)
	binary.BigEndian.PutUint16(header[IDSize+2:], info.Total)
	binary.BigEndian.PutUint32(header[IDSize+4:], uint32(len(data)))

	result := make([]byte, 0, HeaderSize+len(data))
	result = append(result, header...)
	result = append(result, data...)

	return result, nil
}

// Module implements fragment.FragmentModule.
type Module struct{}

// NewModule creates a new fragment module.
func NewModule() *Module { return &Module{} }

// Create creates a Fragment instance.
func (m *Module) Create(config fragment.FragmentConfig) (fragment.Fragment, error) {
	return New(config), nil
}

// Name returns the module name.
func (m *Module) Name() string { return "simple" }

func init() {
	fragment.Register(NewModule())
}
