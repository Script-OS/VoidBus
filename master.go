package VoidBus

import (
	"VoidBus/protocol"
	"VoidBus/serialization"
	"VoidBus/utils"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

type SessionItem struct {
	Info    SessionInfo
	Session protocol.Session

	serialIndex string
}

type Master struct {
	BaseBus
	Protocols      []protocol.Protocol
	Serializations map[string]serialization.Serialization
	Sessions       sync.Map
}

func NewMaster(domain string) *Master {
	return &Master{
		BaseBus: BaseBus{
			DomainName: domain,
			service:    map[string]ActionService{},
		},
		Protocols:      []protocol.Protocol{},
		Serializations: map[string]serialization.Serialization{},
		Sessions:       sync.Map{},
	}
}

func (bus *Master) Start() error {
	// a block function
	for _, p := range bus.Protocols {
		go func(p protocol.Protocol) {
			sessionChan, err := p.Accept()
			if err != nil {
				fmt.Println(err)
				return
			}
			for {
				select {
				case s := <-sessionChan:
					go func(s protocol.Session) {
						err := bus.serviceSession(s)
						if err != nil {
							fmt.Println(err)
						}
						_ = s.Close()
					}(s)
					break
				}
			}
		}(p)
	}
	// TODO: block?
	return nil
}

func (bus *Master) serviceSession(s protocol.Session) error {
	data, err := utils.ReadWithLength(s)
	if err != nil {
		return err
	}
	itemInfo := &SessionInfo{}
	err = json.Unmarshal(data, itemInfo)
	if err != nil {
		return err
	}

	info := SessionInfo{
		DomainName:   bus.DomainName,
		Topics:       []string{},
		SerialScheme: []string{},
	}
	data, err = json.Marshal(&info)
	err = utils.SendWithLength(s, data)
	if err != nil {
		return err
	}

	serialIndex := ""
	// TODO: receive the selected serialization

	bus.Sessions.Store(itemInfo.DomainName, &SessionItem{
		Info:        *itemInfo,
		Session:     s,
		serialIndex: serialIndex,
	})

	for !s.Closed() {
		data, err := utils.ReadWithLength(s)
		if err != nil {
			return err
		}
		msg, err := bus.Serializations[serialIndex].Unmarshal(data)
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
						session:       s,
						serialization: bus.Serializations[serialIndex],
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

func (bus *Master) ActionCall(domain string, topic string, msg any) (yieldChan chan any, retChan chan any, err error) {
	session, ok := bus.Sessions.Load(domain)
	if !ok {
		return nil, nil, errors.New("no such domain")
	}
	found := false
	for _, t := range session.(*SessionItem).Info.Topics {
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
	// TODO: send request
	return handle.yieldChan, handle.returnChan, nil
}
