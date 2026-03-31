package config

import "time"

// Config holds all configuration for the demo
type Config struct {
	// Mode is either "client" or "server"
	Mode string

	// ServerAddr is the server address (host:port)
	ServerAddr string

	// ChannelCount is the number of TCP channels to establish
	ChannelCount int

	// MinSecurityLevel is the minimum acceptable security level
	MinSecurityLevel string

	// TestDataSize is the size of test data to send (in bytes)
	TestDataSize int

	// HandshakeTimeout is the timeout for handshake negotiation
	HandshakeTimeout time.Duration

	// ReadTimeout is the timeout for reading from channels
	ReadTimeout time.Duration
}

// DefaultClientConfig returns a default client configuration
func DefaultClientConfig() *Config {
	return &Config{
		Mode:             "client",
		ServerAddr:       "localhost:8080",
		ChannelCount:     4,
		MinSecurityLevel: "Medium",
		TestDataSize:     50 * 1024, // 50KB
		HandshakeTimeout: 10 * time.Second,
		ReadTimeout:      30 * time.Second,
	}
}

// DefaultServerConfig returns a default server configuration
func DefaultServerConfig() *Config {
	return &Config{
		Mode:             "server",
		ServerAddr:       ":8080",
		ChannelCount:     4,
		MinSecurityLevel: "Medium",
		TestDataSize:     50 * 1024, // 50KB
		HandshakeTimeout: 10 * time.Second,
		ReadTimeout:      30 * time.Second,
	}
}

// GenerateTestData generates readable random text of specified size
func GenerateTestData(size int) []byte {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 "
	data := make([]byte, size)
	for i := range data {
		data[i] = charset[i%len(charset)]
	}
	return data
}
