package main

import (
	"context"
	"io"
	"sync"

	pb "github.com/rexlx/squall/proto"
)

type GrpcServer struct {
	pb.UnimplementedChatServiceServer
	appServer *Server // Reference to your main existing Server struct

	// Track active streams: roomID -> userID -> stream
	streams  map[string]map[string]pb.ChatService_StreamServer
	streamMu sync.RWMutex
}

func NewGrpcServer(app *Server) *GrpcServer {
	return &GrpcServer{
		appServer: app,
		streams:   make(map[string]map[string]pb.ChatService_StreamServer),
	}
}

// 1. Login Implementation
func (s *GrpcServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	// Simple mock logic connecting to your User struct logic
	// In reality, fetch user from s.appServer.DB or memory
	// This mirrors your existing HTTP LoginHandler logic

	// Mock success for demonstration
	return &pb.LoginResponse{
		User: &pb.User{
			Id:        "user-123",
			Email:     req.Email,
			FirstName: "Test",
		},
		Token: "simulated_grpc_token",
		Error: false,
	}, nil
}

// 2. JoinRoom Implementation
func (s *GrpcServer) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.RoomResponse, error) {
	// Logic to find/create room in s.appServer.Rooms
	s.appServer.Memory.Lock()
	defer s.appServer.Memory.Unlock()

	// simplified logic
	roomID := "room-" + req.RoomName

	return &pb.RoomResponse{
		RoomId:  roomID,
		Name:    req.RoomName,
		Success: true,
	}, nil
}

// 3. Stream Implementation (The heavy lifter)
func (s *GrpcServer) Stream(stream pb.ChatService_StreamServer) error {
	// Handshake: Client sends a first message to identify themselves/room?
	// Or we use context metadata. For simplicity, let's wait for the first message.
	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}

	userID := firstMsg.UserId
	roomID := firstMsg.RoomId

	s.registerStream(roomID, userID, stream)
	defer s.deregisterStream(roomID, userID)

	// Broadcast the first message if it contains content
	if firstMsg.MessageContent != "" {
		s.Broadcast(firstMsg)
	}

	// Loop for incoming messages
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Logic to save message to DB could go here
		// s.appServer.DB.StoreMessage(...)

		s.Broadcast(msg)
	}
}

func (s *GrpcServer) registerStream(roomID, userID string, stream pb.ChatService_StreamServer) {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	if _, ok := s.streams[roomID]; !ok {
		s.streams[roomID] = make(map[string]pb.ChatService_StreamServer)
	}
	s.streams[roomID][userID] = stream
}

func (s *GrpcServer) deregisterStream(roomID, userID string) {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	if _, ok := s.streams[roomID]; ok {
		delete(s.streams[roomID], userID)
	}
}

func (s *GrpcServer) Broadcast(msg *pb.ChatMessage) {
	s.streamMu.RLock()
	defer s.streamMu.RUnlock()

	roomStreams, ok := s.streams[msg.RoomId]
	if !ok {
		return
	}

	for _, stream := range roomStreams {
		// Ignore errors for now or handle disconnects
		stream.Send(msg)
	}
}
