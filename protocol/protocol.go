package protocol

import "io"

type Session interface {
	io.ReadWriter
	Close() error
	Closed() bool
}

type Protocol interface {
	Accept() chan Session
	Dial() Session
}
