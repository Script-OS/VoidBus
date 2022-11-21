package serialization

const (
	CommonMessageCall uint32 = iota
	CommonMessageYield
	CommonMessageReturn
)

type CommonMessage struct {
	Type  uint32
	Index uint32
	Topic string // optional, only for call
	Data  any
}

type Serialization interface {
	RegisterType(identity string, v any) error
	Marshal(message CommonMessage) (data []byte, err error)
	Unmarshal(data []byte) (message CommonMessage, err error)
}
