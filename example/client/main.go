package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
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

const serverHost = "127.0.0.1"

func main() {
	key := []byte("voidbus-example-secret-key-32b!!")

	bus, err := voidbus.New(nil)
	if err != nil {
		panic(err)
	}
	bus.SetKey(key)

	// Enable debug mode to track channel/codec usage
	bus.SetDebugMode(true)

	// Register multiple codecs - Bus randomly selects codec chains per message
	bus.RegisterCodec(base64.New())
	bus.RegisterCodec(xor.New())
	bus.RegisterCodec(aes.NewAES256Codec())
	bus.RegisterCodec(chacha20.New())

	// Register ALL channels - Bus uses them all for multi-channel connection
	// Each channel sends NegotiateRequest with the same SessionID
	channels := []struct {
		ch  channel.Channel
		typ string
	}{
		{must(tcp.NewClientChannel(channel.ChannelConfig{
			Address:        fmt.Sprintf("%s:19000", serverHost),
			ConnectTimeout: 5 * time.Second,
		})), "TCP"},
		{must(ws.NewClientChannel(channel.ChannelConfig{
			Address:        fmt.Sprintf("ws://%s:19001", serverHost),
			ConnectTimeout: 5 * time.Second,
		})), "WS"},
		{must(udp.NewClientChannel(channel.ChannelConfig{
			Address:        fmt.Sprintf("%s:19002", serverHost),
			ConnectTimeout: 5 * time.Second,
		})), "UDP"},
	}

	for _, c := range channels {
		bus.AddChannel(c.ch)
		fmt.Printf("Registered %s channel\n", c.typ)
	}

	// Dial uses all registered channels for multi-channel connection
	fmt.Println("Connecting to server...")
	conn, err := bus.Dial()
	if err != nil {
		panic(fmt.Sprintf("dial failed: %v", err))
	}

	fmt.Println("Connected! Messages will be distributed across TCP/WS/UDP channels.")
	fmt.Println("Codec chains: base64 | xor | aes | chacha20 (random selection)")
	fmt.Println("Type messages to send (Ctrl+C to exit):")

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Receive loop
	recvDone := make(chan struct{})
	go func() {
		defer close(recvDone)
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				fmt.Printf("\nConnection closed: %v\n", err)
				return
			}
			fmt.Printf("\n[server] %s\n", string(buf[:n]))
		}
	}()

	// Send loop with channel/codec display
	scanner := bufio.NewScanner(os.Stdin)
	done := make(chan struct{})
	go func() {
		for {
			fmt.Print("client> ")
			if !scanner.Scan() {
				close(done)
				return
			}
			msg := scanner.Text()
			if msg == "" {
				continue
			}

			// Send message
			if _, err := conn.Write([]byte(msg)); err != nil {
				fmt.Printf("Send error: %v\n", err)
				return
			}

			// Display channel/codec info (type assertion to access debug methods)
			// Note: GetLastSendInfo only returns data in debug mode
			if vconn, ok := conn.(interface{ GetLastSendInfo() *voidbus.SendInfo }); ok {
				info := vconn.GetLastSendInfo()
				if info != nil {
					// Format channel distribution
					channelCounts := make(map[string]int)
					for _, chID := range info.Channels {
						// Extract channel type from ID (e.g., "tcp-abc123" -> "TCP")
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

					// Format codec chain
					codecChain := strings.Join(info.CodecChain, "->")
					if codecChain == "" {
						codecChain = "(unknown)"
					}

					fmt.Printf("  Sent via [%s] with codec [%s] (%d fragments, %d bytes)\n",
						chSummary, codecChain, info.FragmentCnt, info.DataSize)
				}
			}
		}
	}()

	select {
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal %v, disconnecting...\n", sig)
	case <-done:
		fmt.Println("\nEOF received, disconnecting...")
	case <-recvDone:
		fmt.Println("Server disconnected.")
	}

	conn.Close()
	bus.Stop()
	fmt.Println("Client stopped.")
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
