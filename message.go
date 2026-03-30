package voidbus

// Message represents a message in the bus
type Message struct {
	// ID is a unique identifier for the message
	ID string

	// Data is the message payload
	Data []byte

	// Metadata contains additional information about the message
	Metadata MessageMetadata
}

// MessageMetadata contains metadata about a message
type MessageMetadata struct {
	// Source is the source of the message (e.g., channel address)
	Source string

	// Timestamp is when the message was created/received
	Timestamp int64

	// EncodingType is the type of encoding used
	EncodingType string

	// FragmentInfo contains fragmentation information if applicable
	FragmentInfo *FragmentMetadata

	// Priority is the message priority (higher = more important)
	Priority int

	// TTL is the time-to-live in seconds (0 = no TTL)
	TTL int
}

// FragmentMetadata contains metadata about fragmentation
type FragmentMetadata struct {
	// ID is the unique identifier for the original data
	ID string

	// Index is the sequence number of this fragment
	Index int

	// TotalCount is the total number of fragments
	TotalCount int

	// IsLast indicates if this is the last fragment
	IsLast bool
}

// MessageHandler processes messages
type MessageHandler interface {
	Handle(message Message) error
}

// MessageHandlerFunc is an adapter to allow using ordinary functions as message handlers
type MessageHandlerFunc func(message Message) error

func (f MessageHandlerFunc) Handle(message Message) error {
	return f(message)
}

// Common errors are defined in errors.go
