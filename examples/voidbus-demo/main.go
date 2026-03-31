package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"voidbus-demo/internal/client"
	"voidbus-demo/internal/config"
	"voidbus-demo/internal/server"
)

const shutdownTimeout = 5 * time.Second

func main() {
	// Parse command line flags
	mode := flag.String("mode", "client", "运行模式：client 或 server")
	addr := flag.String("addr", "", "服务器地址 (client 模式) 或监听地址 (server 模式)")
	channels := flag.Int("channels", 4, "TCP channel 数量")
	dataSize := flag.Int("size", 50*1024, "测试数据大小 (bytes)")
	timeout := flag.Int("timeout", 30, "读取超时时间 (秒)")
	flag.Parse()

	// Create root context for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Signal handler goroutine
	go func() {
		sig := <-sigChan
		log.Printf("收到信号 %v，正在关闭...", sig)
		cancel()
	}()

	var err error
	switch *mode {
	case "client":
		err = runClient(ctx, *addr, *channels, *dataSize, *timeout)
	case "server":
		err = runServer(ctx, *addr, *channels, *timeout)
	default:
		log.Fatalf("未知模式：%s, 请使用 -mode=client 或 -mode=server", *mode)
	}

	if err != nil {
		log.Printf("运行结束: %v", err)
	}
}

func runClient(ctx context.Context, addr string, channels, dataSize, timeout int) error {
	if addr == "" {
		addr = "localhost:8080"
	}

	cfg := config.DefaultClientConfig()
	cfg.ServerAddr = addr
	cfg.ChannelCount = channels
	cfg.TestDataSize = dataSize
	cfg.ReadTimeout = time.Duration(timeout) * time.Second

	log.Printf("Client: 启动配置 - 地址=%s, channels=%d, 数据大小=%d bytes",
		addr, channels, dataSize)

	// Create client
	c := client.NewClient(cfg)

	// Connect
	if err := c.Connect(); err != nil {
		return fmt.Errorf("连接失败：%w", err)
	}

	// Ensure cleanup on exit
	defer func() {
		c.Shutdown(shutdownTimeout)
	}()

	// Start receiving
	c.StartReceiving()

	// Give receivers time to start
	time.Sleep(500 * time.Millisecond)

	// Generate test data
	testData := config.GenerateTestData(dataSize)
	log.Printf("Client: 生成了 %d bytes 测试数据", len(testData))

	// Send data
	if err := c.SendData(testData); err != nil {
		return fmt.Errorf("发送失败：%w", err)
	}

	// Wait for echo response or cancellation
	log.Println("Client: 等待服务器回显数据...")

	resultChan := make(chan []byte, 1)
	errChan := make(chan error, 1)

	go func() {
		received, err := c.ReceiveData()
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- received
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("客户端被中断")
	case err := <-errChan:
		return fmt.Errorf("接收失败：%w", err)
	case received := <-resultChan:
		// Verify data
		if len(received) != len(testData) {
			return fmt.Errorf("数据长度不匹配：期望 %d, 收到 %d", len(testData), len(received))
		}

		for i := range testData {
			if testData[i] != received[i] {
				return fmt.Errorf("数据内容不匹配于位置 %d", i)
			}
		}

		log.Printf("Client: ✓ 成功接收并验证回显数据 (%d bytes)", len(received))
		log.Println("Client: 测试完成！")
		return nil
	}
}

func runServer(ctx context.Context, addr string, channels, timeout int) error {
	if addr == "" {
		addr = ":8080"
	}

	cfg := config.DefaultServerConfig()
	cfg.ServerAddr = addr
	cfg.ChannelCount = channels
	cfg.ReadTimeout = time.Duration(timeout) * time.Second

	log.Printf("Server: 启动配置 - 监听=%s, channels=%d", addr, channels)

	// Create server
	s := server.NewServer(cfg)

	// Start server
	if err := s.Start(); err != nil {
		return fmt.Errorf("启动失败：%w", err)
	}

	// Start shutdown handler goroutine
	// This ensures Shutdown is called when ctx is cancelled,
	// which closes shutdownCh and unblocks ReceiveData
	fmt.Fprintf(os.Stderr, "[DEBUG] Starting shutdown handler goroutine...\n")
	go func() {
		fmt.Fprintf(os.Stderr, "[DEBUG] Shutdown handler waiting for ctx.Done()...\n")
		<-ctx.Done()
		fmt.Fprintf(os.Stderr, "[DEBUG] Shutdown handler triggered, calling Shutdown...\n")
		s.Shutdown(shutdownTimeout)
		fmt.Fprintf(os.Stderr, "[DEBUG] Shutdown handler completed\n")
	}()
	fmt.Fprintf(os.Stderr, "[DEBUG] Shutdown handler goroutine launched\n")

	// Accept connections in background, check for cancellation
	acceptDone := make(chan error, 1)
	go func() {
		acceptDone <- s.AcceptConnections()
	}()

	// Wait for connections or cancellation
	select {
	case <-ctx.Done():
		return fmt.Errorf("服务器被中断")
	case err := <-acceptDone:
		if err != nil {
			return fmt.Errorf("接受连接失败：%w", err)
		}
	}

	// Check if shutdown was triggered during accept
	if s.IsShutdown() {
		return fmt.Errorf("服务器被中断")
	}

	// Start receiving
	s.StartReceiving()

	log.Println("Server: 等待客户端发送数据...")

	// Main server loop - handle requests until cancellation
	for {
		// Check context cancellation first
		select {
		case <-ctx.Done():
			return fmt.Errorf("服务器被中断")
		default:
		}

		// Check shutdown status (may be triggered by Close() failure)
		if s.IsShutdown() {
			return fmt.Errorf("服务器被中断")
		}

		// Wait for data - shutdownCh is closed by Shutdown() goroutine
		received, err := s.ReceiveData()
		if err != nil {
			// Check if this is shutdown signal
			if s.IsShutdown() {
				return fmt.Errorf("服务器被中断")
			}
			// Timeout or other error, continue waiting
			continue
		}

		log.Printf("Server: 收到数据 (%d bytes)", len(received))

		// Echo back
		log.Println("Server: 回显数据给客户端...")
		if err := s.SendData(received); err != nil {
			log.Printf("Server: 发送回显失败：%v", err)
			continue
		}

		log.Println("Server: 回显完成！")
	}
}
