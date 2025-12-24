package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"time"

	pb "github.com/rexlx/squall/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type APIClient struct {
	GrpcClient pb.ChatServiceClient
	Conn       *grpc.ClientConn
	Stream     pb.ChatService_StreamClient

	Token       string
	User        *pb.User
	CurrentRoom *pb.RoomResponse
	MsgChan     chan *pb.ChatMessage
}

var Client = &APIClient{
	MsgChan: make(chan *pb.ChatMessage, 10),
}

func LoadTLSConfig() (*tls.Config, error) {
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

func (c *APIClient) getAuthContext(ctx context.Context) context.Context {
	md := metadata.Pairs("authorization", c.Token)
	return metadata.NewOutgoingContext(ctx, md)
}

func (c *APIClient) Login(email, password string) error {
	// Login is public, no token needed
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

func (c *APIClient) JoinRoom(roomName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// --- FIX: Attach Token ---
	ctx = c.getAuthContext(ctx)
	// -------------------------

	resp, err := c.GrpcClient.JoinRoom(ctx, &pb.JoinRoomRequest{
		Email:    c.User.Email,
		RoomName: roomName,
	})
	if err != nil {
		return err
	}

	c.CurrentRoom = resp

	// Update Local Cache (Deduplicated)
	var cleanHistory []string
	for _, h := range c.User.History {
		if h != roomName {
			cleanHistory = append(cleanHistory, h)
		}
	}
	c.User.History = append(cleanHistory, roomName)

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

	// Restart stream for the new room
	return c.StartStream()
}

func (c *APIClient) StartStream() error {
	// Determine context
	ctx := context.Background()

	// --- FIX: Attach Token ---
	ctx = c.getAuthContext(ctx)
	// -------------------------

	stream, err := c.GrpcClient.Stream(ctx)
	if err != nil {
		return err
	}
	c.Stream = stream

	go func() {
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				fmt.Println("Stream Error:", err)
				return
			}
			c.MsgChan <- msg
		}
	}()

	return nil
}

func (c *APIClient) SendMessage(text string) error {
	enc, err := EncryptMessage(text)
	if err != nil {
		return err
	}

	msg := &pb.ChatMessage{
		UserId:         c.User.Id,
		Email:          c.User.Email,
		RoomId:         c.CurrentRoom.RoomId,
		MessageContent: enc.Data,
		HotSauce:       enc.KeyName,
		Iv:             enc.IV,
		Timestamp:      time.Now().Unix(),
	}

	// Note: Send() uses the stream's context, which we already authenticated in StartStream.
	// However, if stream reconnection logic is added later, ensure it re-authenticates.
	return c.Stream.Send(msg)
}
