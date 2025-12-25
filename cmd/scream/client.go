package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"sync"
	"time"

	pb "github.com/rexlx/squall/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type APIClient struct {
	GrpcClient pb.ChatServiceClient
	Conn       *grpc.ClientConn

	// Map of RoomID -> Stream
	Streams map[string]pb.ChatService_StreamClient
	// Map of RoomID -> CancelFunc (to stop the receiving goroutine/context)
	Cancels map[string]context.CancelFunc
	mu      sync.RWMutex

	Token   string
	User    *pb.User
	MsgChan chan *pb.ChatMessage
}

var Client = &APIClient{
	MsgChan: make(chan *pb.ChatMessage, 100), // Buffered for safety
	Streams: make(map[string]pb.ChatService_StreamClient),
	Cancels: make(map[string]context.CancelFunc),
}

func LoadTLSConfig() (*tls.Config, error) {
	// Adjust path if needed, or use embedded certs
	cert, err := tls.LoadX509KeyPair("data/client-cert.pem", "data/client-key.pem")
	if err != nil {
		return nil, err
	}
	cfh := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
	return cfh, nil
}

func InitClient() error {
	tlsConfig, err := LoadTLSConfig()
	if err != nil {
		return err
	}

	creds := credentials.NewTLS(tlsConfig)
	conn, err := grpc.Dial("localhost:8080", grpc.WithTransportCredentials(creds))
	if err != nil {
		return err
	}

	Client.Conn = conn
	Client.GrpcClient = pb.NewChatServiceClient(conn)
	return nil
}

func (c *APIClient) Login(email, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	resp, err := c.GrpcClient.Login(ctx, &pb.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return err
	}

	if resp.Error {
		return fmt.Errorf("login failed: %s", resp.Message)
	}

	c.User = resp.User
	c.Token = resp.Token
	return nil
}

// Helper to attach JWT
func (c *APIClient) getAuthContext(ctx context.Context) context.Context {
	md := metadata.Pairs("authorization", c.Token)
	return metadata.NewOutgoingContext(ctx, md)
}

// JoinRoom calls the RPC, processes history, and starts the stream
func (c *APIClient) JoinRoom(roomName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	ctx = c.getAuthContext(ctx)

	// 1. Capture the response (instead of _)
	resp, err := c.GrpcClient.JoinRoom(ctx, &pb.JoinRoomRequest{
		Email:    c.User.Email,
		RoomName: roomName,
	})
	if err != nil {
		return err
	}

	// 2. Process History
	// The server sends history (Oldest -> Newest). We push them to the
	// MsgChan just like real-time messages so the UI renders them.
	if len(resp.History) > 0 {
		for _, msg := range resp.History {
			c.MsgChan <- msg
		}
	}

	// 3. Update local cache (Sidebar)
	c.AddRoomToCache(roomName)

	// 4. Start Real-time Stream
	return c.StartStream(roomName)
}

func (c *APIClient) AddRoomToCache(roomName string) {
	exists := false
	for _, r := range c.User.Rooms {
		if r == roomName {
			exists = true
			break
		}
	}
	if !exists {
		c.User.Rooms = append(c.User.Rooms, roomName)
	}
}

// LeaveRoom closes the stream and cancels the context
func (c *APIClient) LeaveRoom(roomName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cancel, ok := c.Cancels[roomName]; ok {
		cancel() // Stop the receive loop
	}
	delete(c.Cancels, roomName)
	delete(c.Streams, roomName)
}

func (c *APIClient) StartStream(roomName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 1. Skip if already connected
	if _, ok := c.Streams[roomName]; ok {
		return nil
	}

	// 2. Create Context with Cancel
	ctx, cancel := context.WithCancel(context.Background())
	ctx = c.getAuthContext(ctx)

	stream, err := c.GrpcClient.Stream(ctx)
	if err != nil {
		cancel()
		return err
	}

	// 3. Handshake: Send first packet with RoomID
	handshake := &pb.ChatMessage{
		UserId:         c.User.Id,
		RoomId:         roomName,
		MessageContent: "", // Empty content = Handshake
	}
	if err := stream.Send(handshake); err != nil {
		cancel()
		return err
	}

	// 4. Store Stream
	c.Streams[roomName] = stream
	c.Cancels[roomName] = cancel

	// 5. Start Receive Loop
	go func(rName string, s pb.ChatService_StreamClient) {
		defer cancel() // Cleanup on exit
		for {
			msg, err := s.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				// Expected error when we close the tab
				if ctx.Err() == context.Canceled {
					return
				}
				fmt.Printf("Stream Error [%s]: %v\n", rName, err)
				return
			}
			c.MsgChan <- msg
		}
	}(roomName, stream)

	return nil
}

func (c *APIClient) SendMessage(roomName, text string) error {
	c.mu.RLock()
	stream, ok := c.Streams[roomName]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("not connected to room %s", roomName)
	}

	enc, err := EncryptMessage(text)
	if err != nil {
		return err
	}

	msg := &pb.ChatMessage{
		UserId:         c.User.Id,
		Email:          c.User.Email,
		RoomId:         roomName,
		MessageContent: enc.Data,
		HotSauce:       enc.KeyName,
		Iv:             enc.IV,
		Timestamp:      time.Now().Unix(),
	}

	return stream.Send(msg)
}
