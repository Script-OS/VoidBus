package utils

import (
	"encoding/binary"
	"io"
)

func SendWithLength(w io.Writer, data []byte) error {
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(len(data)))
	_, err := w.Write(bs)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	return nil
}

func ReadWithLength(r io.Reader) ([]byte, error) {
	bs := make([]byte, 4)
	_, err := io.ReadAtLeast(r, bs, 4)
	if err != nil {
		return nil, err
	}
	size := int(binary.LittleEndian.Uint32(bs))
	data := make([]byte, size)
	_, err = io.ReadAtLeast(r, data, size)
	if err != nil {
		return nil, err
	}
	return data, nil
}
