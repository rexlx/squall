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
	// 	client-cert.pem	client-key.pem
	cert, err := tls.LoadX509KeyPair("data/client-cert.pem", "data/client-key.pem")
	if err != nil {
		return nil, err
	}
	cfh := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true, // For testing only; in production, use proper cert verification
	}
	return cfh, nil

}

func InitClient() error {
	tlsConfig, err := LoadTLSConfig()
	if err != nil {
		return err
	}

	// Create gRPC Transport Credentials
	creds := credentials.NewTLS(tlsConfig)

	// Dial the Server
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

func (c *APIClient) JoinRoom(roomName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	resp, err := c.GrpcClient.JoinRoom(ctx, &pb.JoinRoomRequest{
		Email:    c.User.Email,
		RoomName: roomName,
	})
	if err != nil {
		return err
	}

	c.CurrentRoom = resp

	// Start the stream immediately after joining
	return c.StartStream()
}

func (c *APIClient) StartStream() error {
	// Establish the bidirectional stream
	stream, err := c.GrpcClient.Stream(context.Background())
	if err != nil {
		return err
	}
	c.Stream = stream

	// Start listening routine
	go func() {
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return // Stream closed
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
	enc, err := EncryptMessage(text) // Your existing crypto.go logic
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

	return c.Stream.Send(msg)
}
