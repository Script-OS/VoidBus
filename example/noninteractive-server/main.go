// Package main provides a non-interactive VoidBus server for file transfer testing.
package main

import (
	"crypto/sha256"
	"encoding/hex"
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

const key = "voidbus-file-transfer-test-key!!"

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("=== VoidBus Non-Interactive Server ===")
	log.Println("Starting server...")

	// Create bus with larger buffer for file transfer
	config := voidbus.DefaultBusConfig()
	config.RecvBufferSize = 1000 // Increase buffer for large file transfer
	bus, err := voidbus.New(config)
	if err != nil {
		log.Fatalf("Failed to create bus: %v", err)
	}

	if err := bus.SetKey([]byte(key)); err != nil {
		log.Fatalf("Failed to set key: %v", err)
	}
	log.Printf("Encryption key set: %d bytes", len(key))

	if err := bus.SetMaxCodecDepth(3); err != nil {
		log.Fatalf("Failed to set max codec depth: %v", err)
	}
	log.Println("Max codec depth: 3")

	// Disable debug mode to reduce log noise
	bus.SetDebugMode(false)

	bus.RegisterCodec(base64.New())
	bus.RegisterCodec(xor.New())
	bus.RegisterCodec(aes.NewAES256Codec())
	bus.RegisterCodec(chacha20.New())
	log.Println("Registered codecs: base64, xor, aes, chacha20")

	tcpServer := mustChannel(tcp.NewServerChannel(channel.ChannelConfig{Address: fmt.Sprintf(":%d", tcpPort)}))
	wsServer := mustChannel(ws.NewServerChannel(channel.ChannelConfig{Address: fmt.Sprintf(":%d", wsPort)}))
	udpServer := mustChannel(udp.NewServerChannel(channel.ChannelConfig{Address: fmt.Sprintf(":%d", udpPort)}))

	bus.AddChannel(tcpServer)
	bus.AddChannel(wsServer)
	bus.AddChannel(udpServer)

	log.Printf("Server channels: TCP:%d, WS:%d, UDP:%d", tcpPort, wsPort, udpPort)

	listener, err := bus.Listen()
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	log.Println("Waiting for client connection...")

	conn, err := listener.Accept()
	if err != nil {
		log.Fatalf("Failed to accept: %v", err)
	}
	defer conn.Close()

	log.Printf("Client connected: %s", conn.RemoteAddr())
	log.Println("")

	// === Phase 1: Receive file from client ===
	log.Println("=== Phase 1: Receiving file from client ===")

	sizeBuf := make([]byte, 8)
	_, err = io.ReadFull(conn, sizeBuf)
	if err != nil {
		log.Fatalf("Failed to receive file size: %v", err)
	}
	fileSize := int64(sizeBuf[0])<<56 | int64(sizeBuf[1])<<48 | int64(sizeBuf[2])<<40 | int64(sizeBuf[3])<<32 |
		int64(sizeBuf[4])<<24 | int64(sizeBuf[5])<<16 | int64(sizeBuf[6])<<8 | int64(sizeBuf[7])
	log.Printf("Incoming file size: %d bytes (%.2f MB)", fileSize, float64(fileSize)/1024/1024)

	// Receive file data with hash calculation
	receivedFile, err := os.Create("received_file.bin")
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer receivedFile.Close()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(receivedFile, hasher)

	startTime := time.Now()
	received, err := io.CopyN(multiWriter, conn, fileSize)
	if err != nil {
		log.Fatalf("Failed to receive file: %v", err)
	}
	recvDuration := time.Since(startTime)

	receivedHash := hex.EncodeToString(hasher.Sum(nil))

	log.Printf("File received: %d bytes in %v (%.2f MB/s)", received, recvDuration, float64(received)/1024/1024/recvDuration.Seconds())

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

	testFile := "test_file.bin"
	fileInfo, err := os.Stat(testFile)
	if err != nil {
		log.Printf("%s not found, creating a 10MB test file...", testFile)
		createTestFile(testFile, 10*1024*1024)
		fileInfo, _ = os.Stat(testFile)
	}

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

	// Set write deadline for large file transfer
	conn.SetWriteDeadline(time.Now().Add(5 * time.Minute))

	// Send file content in chunks with progress logging
	// Use larger chunks (512KB) to reduce number of messages
	// VoidBus will handle fragmentation internally
	startTime = time.Now()
	buf := make([]byte, 512*1024) // 512KB chunks
	totalSent := int64(0)

	for totalSent < sendFileSize {
		remaining := sendFileSize - totalSent
		toRead := int64(len(buf))
		if remaining < toRead {
			toRead = remaining
		}

		n, err := sendFile.Read(buf[:toRead])
		if err != nil {
			log.Fatalf("Failed to read file: %v", err)
		}

		wrote, err := conn.Write(buf[:n])
		if err != nil {
			log.Fatalf("Failed to send file chunk at offset %d: %v", totalSent, err)
		}
		totalSent += int64(wrote)

		// Log progress every 1MB
		if totalSent%(1024*1024) == 0 || totalSent == sendFileSize {
			elapsed := time.Since(startTime)
			rate := float64(totalSent) / 1024 / 1024 / elapsed.Seconds()
			log.Printf("Progress: %d/%d bytes (%.1f%%, %.2f MB/s)",
				totalSent, sendFileSize, float64(totalSent)/float64(sendFileSize)*100, rate)
		}
	}

	sendDuration := time.Since(startTime)

	log.Printf("File sent: %d bytes in %v (%.2f MB/s)", totalSent, sendDuration, float64(totalSent)/1024/1024/sendDuration.Seconds())

	if vconn, ok := conn.(interface{ GetLastSendInfo() *voidbus.SendInfo }); ok {
		info := vconn.GetLastSendInfo()
		if info != nil {
			printSendInfo(info, "Server send")
		}
	}

	log.Println("")
	log.Println("=== Transfer complete ===")
	log.Printf("Received: received_file.bin (%d bytes)", fileSize)
	log.Printf("  SHA256: %s", receivedHash)
	log.Printf("Sent:     test_file.bin (%d bytes)", sendFileSize)
	log.Printf("  SHA256: %s", sendHash)

	conn.Close()
	bus.Stop()
	log.Println("Server exited successfully")
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
	channelCounts := make(map[string]int)
	for _, chID := range info.Channels {
		chType := strings.Split(chID, "-")[0]
		channelCounts[strings.ToUpper(chType)]++
	}

	chSummary := ""
	for chType, count := range channelCounts {
		if chSummary != "" {
			chSummary += ", "
		}
		chSummary += fmt.Sprintf("%s:%d", chType, count)
	}

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
