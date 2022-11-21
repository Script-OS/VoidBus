package plain

import (
	"VoidBus/serialization"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"reflect"
)

type Marshaler func(v any) ([]byte, error)
type Unmarshaler func(data []byte, v any) error

var WrongFormat = errors.New("wrong format")

func Combine(message serialization.CommonMessage, typeName string, processor Marshaler) (data []byte, err error) {
	body, err := processor(message.Data)
	if err != nil {
		return nil, err
	}
	data = make([]byte, 4)
	binary.LittleEndian.PutUint32(data, message.Index)
	if message.Type == serialization.CommonMessageCall {
		data = append(data, []byte(message.Topic)...)
		data = append(data, byte(0))
	} else {
		data = append(data, byte(message.Type))
	}
	data = append(data, []byte(typeName)...)
	data = append(data, byte(0))
	return append(data, body...), nil
}

func Separate(data []byte, typeTable map[string]reflect.Type, processor Unmarshaler) (message serialization.CommonMessage, err error) {
	message.Index = uint32(data[0])
	if data[1] < 2 {
		message.Type = uint32(data[1])
		message.Topic = ""
		data = data[2:]
	} else {
		before, after, found := bytes.Cut(data[1:], []byte{0})
		if !found {
			return message, WrongFormat
		}
		message.Type = serialization.CommonMessageCall
		message.Topic = string(before)
		data = after
	}
	before, after, found := bytes.Cut(data[1:], []byte{0})
	if !found {
		return message, WrongFormat
	}
	typeItem, ok := typeTable[string(before)]
	if !ok {
		return message, WrongFormat
	}
	holder := reflect.New(typeItem)
	err = processor(after, holder)
	if err != nil {
		return message, err
	}
	message.Data = holder
	return message, nil
}

type Plain struct {
	typeTable map[string]reflect.Type
}

func (s *Plain) RegisterType(identity string, v any) error {
	s.typeTable[identity] = reflect.TypeOf(v)
	return nil
}

func (s *Plain) Marshal(message serialization.CommonMessage) (data []byte, err error) {
	dataType := reflect.TypeOf(message.Data)
	for k, v := range s.typeTable {
		if v == dataType {
			return Combine(message, k, json.Marshal)
		}
	}
	return nil, WrongFormat
}

func (s *Plain) Unmarshal(data []byte) (message serialization.CommonMessage, err error) {
	return Separate(data, s.typeTable, json.Unmarshal)
}
