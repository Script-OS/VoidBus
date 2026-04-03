// Package main provides a non-interactive VoidBus server for file transfer testing.
//
// Usage:
//
//	go run server.go
//
// The server will:
// 1. Listen on TCP:19000, WS:19001, UDP:19002
// 2. Accept one client connection
// 3. Receive a file (saved as received_file.bin)
// 4. Send a file back (test_file.bin from current directory)
// 5. Display detailed logs with channel/codec information
// 6. Exit after transfer completes
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	voidbus "github.com/Script-OS/VoidBus"
	"github.com/Script-OS/VoidBus/channel"
	"github.com/Script-OS/VoidBus/channel/tcp"
	"github.com/Script-OS/VoidBus/channel/udp"
	"github.com/Script-OS/VoidBus/channel/ws"
	"github.com/Script-OS/VoidBus/codec/aes"
	"github.com/Script-OS/VoidBus/codec/base64"
	"github.com/Script-OS/VoidBus/codec/chacha20"
	"github.com/Script-OS/VoidBus/codec/xor"
)

const (
	tcpPort = 19000
	wsPort  = 19001
	udpPort = 19002
)

const key = "voidbus-file-transfer-test-key-32!"

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("=== VoidBus Non-Interactive Server ===")
	log.Println("Starting server...")

	// Create bus
	bus, err := voidbus.New(nil)
	if err != nil {
		log.Fatalf("Failed to create bus: %v", err)
	}

	// Set key (must match client)
	if err := bus.SetKey([]byte(key)); err != nil {
		log.Fatalf("Failed to set key: %v", err)
	}
	log.Printf("Encryption key set: %d bytes", len(key))

	// Limit codec depth to 3
	if err := bus.SetMaxCodecDepth(3); err != nil {
		log.Fatalf("Failed to set max codec depth: %v", err)
	}
	log.Println("Max codec depth: 3")

	// Enable debug mode for detailed logs
	bus.SetDebugMode(true)
	log.Println("Debug mode: enabled")

	// Register codecs
	bus.RegisterCodec(base64.New())
	bus.RegisterCodec(xor.New())
	bus.RegisterCodec(aes.NewAES256Codec())
	bus.RegisterCodec(chacha20.New())
	log.Println("Registered codecs: base64, xor, aes, chacha20")

	// Add server channels
	tcpServer := mustChannel(tcp.NewServerChannel(channel.ChannelConfig{Address: fmt.Sprintf(":%d", tcpPort)}))
	wsServer := mustChannel(ws.NewServerChannel(channel.ChannelConfig{Address: fmt.Sprintf(":%d", wsPort)}))
	udpServer := mustChannel(udp.NewServerChannel(channel.ChannelConfig{Address: fmt.Sprintf(":%d", udpPort)}))

	bus.AddChannel(tcpServer)
	bus.AddChannel(wsServer)
	bus.AddChannel(udpServer)

	log.Printf("Server channels: TCP:%d, WS:%d, UDP:%d", tcpPort, wsPort, udpPort)

	// Start listening
	listener, err := bus.Listen()
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	log.Println("Waiting for client connection...")

	// Accept one connection
	conn, err := listener.Accept()
	if err != nil {
		log.Fatalf("Failed to accept: %v", err)
	}
	defer conn.Close()

	log.Printf("Client connected: %s", conn.RemoteAddr())
	log.Println("")

	// === Phase 1: Receive file from client ===
	log.Println("=== Phase 1: Receiving file from client ===")

	// First, receive file size (8 bytes)
	sizeBuf := make([]byte, 8)
	_, err = io.ReadFull(conn, sizeBuf)
	if err != nil {
		log.Fatalf("Failed to receive file size: %v", err)
	}
	fileSize := int64(sizeBuf[0])<<56 | int64(sizeBuf[1])<<48 | int64(sizeBuf[2])<<40 | int64(sizeBuf[3])<<32 |
		int64(sizeBuf[4])<<24 | int64(sizeBuf[5])<<16 | int64(sizeBuf[6])<<8 | int64(sizeBuf[7])
	log.Printf("Incoming file size: %d bytes (%.2f MB)", fileSize, float64(fileSize)/1024/1024)

	// Receive file data
	receivedFile, err := os.Create("received_file.bin")
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer receivedFile.Close()

	startTime := time.Now()
	received, err := io.CopyN(receivedFile, conn, fileSize)
	if err != nil {
		log.Fatalf("Failed to receive file: %v", err)
	}
	recvDuration := time.Since(startTime)

	log.Printf("File received: %d bytes in %v", received, recvDuration)
	log.Printf("Receive rate: %.2f MB/s", float64(received)/1024/1024/recvDuration.Seconds())

	// Get send info from client (via debug mode)
	if vconn, ok := conn.(interface{ GetLastSendInfo() *voidbus.SendInfo }); ok {
		info := vconn.GetLastSendInfo()
		if info != nil {
			printSendInfo(info, "Client send")
		}
	}

	receivedFile.Close()
	log.Printf("File saved: received_file.bin")
	log.Println("")

	// === Phase 2: Send file to client ===
	log.Println("=== Phase 2: Sending file to client ===")

	// Check if test file exists
	testFile := "test_file.bin"
	fileInfo, err := os.Stat(testFile)
	if err != nil {
		log.Printf("Warning: %s not found, creating a 10MB test file...", testFile)
		createTestFile(testFile, 10*1024*1024)
		fileInfo, _ = os.Stat(testFile)
	}

	// Open file to send
	sendFile, err := os.Open(testFile)
	if err != nil {
		log.Fatalf("Failed to open test file: %v", err)
	}
	defer sendFile.Close()

	sendFileSize := fileInfo.Size()
	log.Printf("File to send: %s (%d bytes, %.2f MB)", testFile, sendFileSize, float64(sendFileSize)/1024/1024)

	// Send file size first (8 bytes, big-endian)
	sizeBuf = make([]byte, 8)
	sizeBuf[0] = byte(sendFileSize >> 56)
	sizeBuf[1] = byte(sendFileSize >> 48)
	sizeBuf[2] = byte(sendFileSize >> 40)
	sizeBuf[3] = byte(sendFileSize >> 32)
	sizeBuf[4] = byte(sendFileSize >> 24)
	sizeBuf[5] = byte(sendFileSize >> 16)
	sizeBuf[6] = byte(sendFileSize >> 8)
	sizeBuf[7] = byte(sendFileSize)

	if _, err := conn.Write(sizeBuf); err != nil {
		log.Fatalf("Failed to send file size: %v", err)
	}
	log.Printf("Sent file size header: %d bytes", sendFileSize)

	// Send file content
	startTime = time.Now()
	sent, err := io.CopyN(conn, sendFile, sendFileSize)
	if err != nil {
		log.Fatalf("Failed to send file: %v", err)
	}
	sendDuration := time.Since(startTime)

	log.Printf("File sent: %d bytes in %v", sent, sendDuration)
	log.Printf("Send rate: %.2f MB/s", float64(sent)/1024/1024/sendDuration.Seconds())

	// Get our send info (server's send)
	if vconn, ok := conn.(interface{ GetLastSendInfo() *voidbus.SendInfo }); ok {
		info := vconn.GetLastSendInfo()
		if info != nil {
			printSendInfo(info, "Server send")
		}
	}

	log.Println("")
	log.Println("=== Transfer complete ===")
	log.Printf("Received: received_file.bin (%d bytes)", fileSize)
	log.Printf("Sent: test_file.bin (%d bytes)", sendFileSize)
	log.Println("Server exiting...")
}

func mustChannel(ch channel.Channel, err error) channel.Channel {
	if err != nil {
		log.Fatalf("Failed to create channel: %v", err)
	}
	return ch
}

func createTestFile(path string, size int64) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("Failed to create test file: %v", err)
	}
	defer f.Close()

	// Write pattern
	buf := make([]byte, 64*1024)
	for i := range buf {
		buf[i] = byte(i % 256)
	}

	written := int64(0)
	for written < size {
		remaining := size - written
		if remaining < int64(len(buf)) {
			f.Write(buf[:remaining])
			written += remaining
		} else {
			f.Write(buf)
			written += int64(len(buf))
		}
	}
}

func printSendInfo(info *voidbus.SendInfo, prefix string) {
	// Count channel usage
	channelCounts := make(map[string]int)
	for _, chID := range info.Channels {
		chType := strings.Split(chID, "-")[0]
		channelCounts[strings.ToUpper(chType)]++
	}

	// Format channel summary
	chSummary := ""
	for chType, count := range channelCounts {
		if chSummary != "" {
			chSummary += ", "
		}
		chSummary += fmt.Sprintf("%s:%d", chType, count)
	}

	// Format codec chain
	codecChain := strings.Join(info.CodecChain, "->")
	if codecChain == "" {
		codecChain = "(unknown)"
	}

	log.Printf("%s info:", prefix)
	log.Printf("  Channels: [%s]", chSummary)
	log.Printf("  Codec:    [%s]", codecChain)
	log.Printf("  Fragments: %d", info.FragmentCnt)
	log.Printf("  Data size: %d bytes", info.DataSize)
}
