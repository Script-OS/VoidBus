package BasicTCP

import (
	"VoidBus/protocol"
	"log"
	"net"
)

type Session struct {
	conn   *net.TCPConn
	closed bool
}

func (s *Session) Read(p []byte) (n int, err error) {
	n, err = s.conn.Read(p)
	if err == net.ErrClosed {
		s.closed = true
	}
	return n, err
}

func (s *Session) Write(p []byte) (n int, err error) {
	n, err = s.conn.Write(p)
	if err == net.ErrClosed {
		s.closed = true
	}
	return n, err
}

func (s *Session) Close() error {
	return s.conn.Close()
}

func (s *Session) Closed() bool {
	return s.closed
}

type Config struct {
	ListenNetwork string
	ListenAddress *net.TCPAddr
	DialNetwork   string
	DialAddress   *net.TCPAddr
}

type instance struct {
	config Config
}

func New(config Config) protocol.Protocol {
	return &instance{config: config}
}

func (p *instance) Accept() (chan protocol.Session, error) {
	listener, err := net.ListenTCP(p.config.ListenNetwork, p.config.ListenAddress)
	if err != nil {
		return nil, err
	}
	ret := make(chan protocol.Session)
	go func() {
		for {
			conn, err := listener.AcceptTCP()
			if err != nil {
				log.Println(err) // TODO:
				break
			}
			ret <- &Session{
				conn:   conn,
				closed: false,
			}
		}
	}()
	return ret, nil
}

func (p *instance) Dial() (protocol.Session, error) {
	conn, err := net.DialTCP(p.config.DialNetwork, nil, p.config.DialAddress)
	if err != nil {
		return nil, err
	}
	return &Session{
		conn:   conn,
		closed: false,
	}, nil
}
