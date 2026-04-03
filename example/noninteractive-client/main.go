// Package main provides a non-interactive VoidBus client for file transfer testing.
//
// Usage:
//
//	go run client.go [server_host]
//
// The client will:
// 1. Connect to server via TCP/WS/UDP
// 2. Send a file (test_file.bin from current directory)
// 3. Wait for server acknowledgment
// 4. Receive a file back (saved as received_file.bin)
// 5. Send acknowledgment to server
// 6. Display detailed logs with channel/codec information
// 7. Exit after transfer completes
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
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

// TransferCompleteMagic is the magic number for transfer completion acknowledgment.
// Fixed 8 bytes for reliable ReadFull.
const TransferCompleteMagic = "DONE1234"

// sendTransferComplete sends a transfer completion acknowledgment.
// Uses a fixed 8-byte magic number for reliable transmission.
func sendTransferComplete(conn net.Conn) error {
	magic := []byte(TransferCompleteMagic)
	_, err := conn.Write(magic)
	return err
}

// waitTransferComplete waits for transfer completion acknowledgment using channel notification.
// Returns error on timeout or invalid magic received.
func waitTransferComplete(conn net.Conn, timeout time.Duration) error {
	done := make(chan error, 1)

	go func() {
		buf := make([]byte, 8)
		_, err := io.ReadFull(conn, buf)
		if err != nil {
			done <- err
			return
		}
		if string(buf) != TransferCompleteMagic {
			done <- errors.New("invalid transfer complete magic")
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return errors.New("timeout waiting for transfer complete")
	}
}

const (
	tcpPort = 19000
	wsPort  = 19001
	udpPort = 19002
)

const key = "voidbus-file-transfer-test-key!!"

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("=== VoidBus Non-Interactive Client ===")

	serverHost := "127.0.0.1"
	if len(os.Args) > 1 {
		serverHost = os.Args[1]
	}
	log.Printf("Server: %s", serverHost)

	// Create bus with larger buffer for file transfer
	config := voidbus.DefaultBusConfig()
	config.RecvBufferSize = 1000 // Increase buffer for large file transfer
	bus, err := voidbus.New(config)
	if err != nil {
		log.Fatalf("Failed to create bus: %v", err)
	}
	log.Printf("Server: %s", serverHost)

	// Set key (must match server)
	if err := bus.SetKey([]byte(key)); err != nil {
		log.Fatalf("Failed to set key: %v", err)
	}
	log.Printf("Encryption key set: %d bytes", len(key))

	// Limit codec depth to 3
	if err := bus.SetMaxCodecDepth(3); err != nil {
		log.Fatalf("Failed to set max codec depth: %v", err)
	}
	log.Println("Max codec depth: 3")

	// Disable debug mode to reduce log noise
	bus.SetDebugMode(false)

	// Register codecs
	bus.RegisterCodec(base64.New())
	bus.RegisterCodec(xor.New())
	bus.RegisterCodec(aes.NewAES256Codec())
	bus.RegisterCodec(chacha20.New())
	log.Println("Registered codecs: base64, xor, aes, chacha20")

	// Add client channels
	connectTimeout := 5 * time.Second
	tcpClient := mustChannel(tcp.NewClientChannel(channel.ChannelConfig{
		Address:        fmt.Sprintf("%s:%d", serverHost, tcpPort),
		ConnectTimeout: connectTimeout,
	}))
	wsClient := mustChannel(ws.NewClientChannel(channel.ChannelConfig{
		Address:        fmt.Sprintf("ws://%s:%d", serverHost, wsPort),
		ConnectTimeout: connectTimeout,
	}))
	udpClient := mustChannel(udp.NewClientChannel(channel.ChannelConfig{
		Address:        fmt.Sprintf("%s:%d", serverHost, udpPort),
		ConnectTimeout: connectTimeout,
	}))

	bus.AddChannel(tcpClient)
	bus.AddChannel(wsClient)
	bus.AddChannel(udpClient)

	log.Printf("Client channels: TCP:%d, WS:%d, UDP:%d", tcpPort, wsPort, udpPort)
	log.Println("Connecting to server...")

	// Dial
	conn, err := bus.Dial()
	if err != nil {
		log.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	log.Printf("Connected to server: %s", conn.RemoteAddr())
	log.Println("")

	// === Phase 1: Send file to server ===
	log.Println("=== Phase 1: Sending file to server ===")

	// Check if test file exists
	testFile := "test_file.bin"
	fileInfo, err := os.Stat(testFile)
	if err != nil {
		log.Printf("%s not found, creating a 10MB test file...", testFile)
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

	// Calculate hash before sending
	sendHasher := sha256.New()
	if _, err := io.Copy(sendHasher, sendFile); err != nil {
		log.Fatalf("Failed to hash file: %v", err)
	}
	sendHash := hex.EncodeToString(sendHasher.Sum(nil))

	// Reset file position for actual sending
	sendFile.Seek(0, 0)

	// Send file size first (8 bytes, big-endian)
	sizeBuf := make([]byte, 8)
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
	startTime := time.Now()
	sent, err := io.CopyN(conn, sendFile, sendFileSize)
	if err != nil {
		log.Fatalf("Failed to send file: %v", err)
	}
	sendDuration := time.Since(startTime)

	log.Printf("File sent: %d bytes in %v", sent, sendDuration)
	log.Printf("Send rate: %.2f MB/s", float64(sent)/1024/1024/sendDuration.Seconds())

	// Get send info (client's send)
	if vconn, ok := conn.(interface{ GetLastSendInfo() *voidbus.SendInfo }); ok {
		info := vconn.GetLastSendInfo()
		if info != nil {
			printSendInfo(info, "Client send")
		}
	}

	// Wait for server's transfer complete acknowledgment using channel notification
	log.Println("Waiting for server transfer complete acknowledgment...")
	if err := waitTransferComplete(conn, 5*time.Minute); err != nil {
		log.Fatalf("Failed to receive transfer complete from server: %v", err)
	}
	log.Println("Server transfer complete acknowledgment received")

	log.Println("")

	// === Phase 2: Receive file from server ===
	log.Println("=== Phase 2: Receiving file from server ===")

	// Receive file size (8 bytes)
	_, err = io.ReadFull(conn, sizeBuf)
	if err != nil {
		log.Fatalf("Failed to receive file size: %v", err)
	}
	fileSize := int64(sizeBuf[0])<<56 | int64(sizeBuf[1])<<48 | int64(sizeBuf[2])<<40 | int64(sizeBuf[3])<<32 |
		int64(sizeBuf[4])<<24 | int64(sizeBuf[5])<<16 | int64(sizeBuf[6])<<8 | int64(sizeBuf[7])
	log.Printf("Incoming file size: %d bytes (%.2f MB)", fileSize, float64(fileSize)/1024/1024)

	// Set read deadline for large file transfer
	conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

	// Receive file data with hash calculation
	receivedFile, err := os.Create("received_file.bin")
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer receivedFile.Close()

	hasher := sha256.New()

	// Use large buffer for VoidBus message-oriented reads
	// Each Read() returns a complete message, which could be large
	buf := make([]byte, 1024*1024) // 1MB buffer
	totalReceived := int64(0)

	startTime = time.Now()
	for totalReceived < fileSize {
		n, err := conn.Read(buf)
		if err != nil {
			log.Fatalf("Failed to receive file: %v", err)
		}

		// Write to file and hasher
		if _, err := receivedFile.Write(buf[:n]); err != nil {
			log.Fatalf("Failed to write to file: %v", err)
		}
		hasher.Write(buf[:n])
		totalReceived += int64(n)

		// Log progress every 1MB
		if totalReceived%(1024*1024) == 0 {
			elapsed := time.Since(startTime)
			rate := float64(totalReceived) / 1024 / 1024 / elapsed.Seconds()
			log.Printf("Progress: %d/%d bytes (%.1f%%, %.2f MB/s)",
				totalReceived, fileSize, float64(totalReceived)/float64(fileSize)*100, rate)
		}
	}
	recvDuration := time.Since(startTime)

	receivedHash := hex.EncodeToString(hasher.Sum(nil))

	log.Printf("File received: %d bytes in %v (%.2f MB/s)", totalReceived, recvDuration, float64(totalReceived)/1024/1024/recvDuration.Seconds())

	// Get server's send info
	if vconn, ok := conn.(interface{ GetLastSendInfo() *voidbus.SendInfo }); ok {
		info := vconn.GetLastSendInfo()
		if info != nil {
			printSendInfo(info, "Server send")
		}
	}

	receivedFile.Close()
	log.Printf("File saved: received_file.bin")

	// Send transfer complete acknowledgment to server
	log.Println("Sending transfer complete acknowledgment...")
	if err := sendTransferComplete(conn); err != nil {
		log.Fatalf("Failed to send transfer complete: %v", err)
	}
	log.Println("Transfer complete acknowledgment sent")

	log.Println("")

	log.Println("=== Transfer complete ===")
	log.Printf("Sent:     test_file.bin (%d bytes)", sendFileSize)
	log.Printf("  SHA256: %s", sendHash)
	log.Printf("Received: received_file.bin (%d bytes)", fileSize)
	log.Printf("  SHA256: %s", receivedHash)

	// Explicitly close connection and stop bus to ensure clean exit
	conn.Close()
	bus.Stop()
	log.Println("Client exited successfully")
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
