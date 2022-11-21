package VoidBus

import (
	"VoidBus/protocol"
	"VoidBus/serialization"
	"VoidBus/utils"
	"sync"
)

type ActionHandler interface {
	Yield(msg any) error
	Return(msg any) error
}

type ActionService func(msg any, handler ActionHandler) error

//type FallbackService func(domain string, topic string, msg []byte, handler ActionHandler) error

type BaseBus struct {
	DomainName string
	service    map[string]ActionService

	callIndex    uint32
	callHandlers sync.Map
}

type SessionInfo struct {
	DomainName   string   `json:"domainName"`
	Topics       []string `json:"topics"` // TODO: map[string]uuid//type-uuid
	SerialScheme []string `json:"serialScheme"`
}

type callActionHandler struct {
	yieldChan  chan any
	returnChan chan any
}

func (h *callActionHandler) Yield(msg any) error {
	// TODO:
	go func() { h.yieldChan <- msg }()
	return nil
}

func (h *callActionHandler) Return(msg any) error {
	// TODO:
	go func() { h.returnChan <- msg }()
	return nil
}

type serviceActionHandler struct {
	index         uint32
	session       protocol.Session
	serialization serialization.Serialization
}

func (h *serviceActionHandler) Yield(msg any) error {
	data, err := h.serialization.Marshal(serialization.CommonMessage{
		Type:  serialization.CommonMessageYield,
		Index: h.index,
		Topic: "",
		Data:  msg,
	})
	if err != nil {
		return err
	}
	return utils.SendWithLength(h.session, data)
}

func (h *serviceActionHandler) Return(msg any) error {
	data, err := h.serialization.Marshal(serialization.CommonMessage{
		Type:  serialization.CommonMessageReturn,
		Index: h.index,
		Topic: "",
		Data:  msg,
	})
	if err != nil {
		return err
	}
	return utils.SendWithLength(h.session, data)
}
