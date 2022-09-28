package VoidBus

import (
	p "VoidBus/protocol"
	s "VoidBus/serialization"
	"sync"
)

type VoidBus struct {
	serialization s.Serialization
	protocol      p.Protocol
	handler       MsgHandler
}

type MsgHandler struct {
	flagOnce      bool
	async         bool
	transactional bool
	sync.Mutex    // lock for an event handler - useful for running async callbacks serially
}
