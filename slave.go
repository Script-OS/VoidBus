package VoidBus

import (
	"VoidBus/protocol"
	"VoidBus/serialization"
	"VoidBus/utils"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
)

type Slave struct {
	BaseBus
	//Fallback       FallbackService
	masterInfo     *SessionInfo
	session        protocol.Session
	serializations []serialization.Serialization

	serialIndex int
}

func NewSlave(domain string, serializations []serialization.Serialization) *Slave {
	return &Slave{
		BaseBus: BaseBus{
			DomainName: domain,
			service:    map[string]ActionService{},
		},
		//Fallback:       nil,
		masterInfo:     nil,
		session:        nil,
		serializations: serializations,
	}
}

func (bus *BaseBus) RegisterService(topic string, service ActionService) {
	bus.service[topic] = service
}

func (bus *Slave) Start(conn protocol.Session) error {
	// a block function
	bus.session = conn
	defer func() { bus.session = nil }()

	info := SessionInfo{
		DomainName:   bus.DomainName,
		Topics:       []string{},
		SerialScheme: []string{},
	}
	data, err := json.Marshal(&info)
	if err != nil {
		return err
	}
	err = utils.SendWithLength(bus.session, data)
	if err != nil {
		return err
	}
	data, err = utils.ReadWithLength(bus.session)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, bus.masterInfo)
	if err != nil {
		return err
	}

	// TODO: select the serialization and send

	for !bus.session.Closed() {
		data, err := utils.ReadWithLength(bus.session)
		if err != nil {
			return err
		}
		msg, err := bus.serializations[bus.serialIndex].Unmarshal(data)
		if err != nil {
			return err
		}
		switch msg.Type {
		case serialization.CommonMessageCall:
			service, ok := bus.service[msg.Topic]
			if !ok {
				fmt.Println("no such service")
			} else {
				go func() {
					err := service(msg.Data, &serviceActionHandler{
						index:         msg.Index,
						session:       bus.session,
						serialization: bus.serializations[bus.serialIndex],
					})
					if err != nil {
						fmt.Println(err)
					}
				}()
			}
			break
		case serialization.CommonMessageYield:
			handler, ok := bus.callHandlers.Load(msg.Index)
			if !ok {
				fmt.Println("no such call-handle")
			} else {
				err := handler.(ActionHandler).Yield(msg.Data)
				if err != nil {
					fmt.Println(err)
				}
			}
			break
		case serialization.CommonMessageReturn:
			handler, ok := bus.callHandlers.LoadAndDelete(msg.Index)
			if !ok {
				fmt.Println("no such call-handle")
			} else {
				err := handler.(ActionHandler).Return(msg.Data)
				if err != nil {
					fmt.Println(err)
				}
			}
			break
		}
	}
	return nil
}

func (bus *Slave) ActionCall(topic string, msg any) (yieldChan chan any, retChan chan any, err error) {
	found := false
	for _, t := range bus.masterInfo.Topics {
		if t == topic {
			found = true
			break
		}
	}
	if !found {
		return nil, nil, errors.New("no such topic")
	}

	callIndex := atomic.AddUint32(&bus.callIndex, 1)
	handle := &callActionHandler{
		yieldChan:  make(chan any),
		returnChan: make(chan any),
	}
	bus.callHandlers.Store(callIndex, handle)

	data, err := bus.serializations[bus.serialIndex].Marshal(serialization.CommonMessage{
		Type:  serialization.CommonMessageCall,
		Index: callIndex,
		Topic: topic,
		Data:  msg,
	})
	if err != nil {
		bus.callHandlers.Delete(callIndex)
		return nil, nil, err
	}
	err = utils.SendWithLength(bus.session, data)
	if err != nil {
		return nil, nil, err
	}
	return handle.yieldChan, handle.returnChan, nil
}
