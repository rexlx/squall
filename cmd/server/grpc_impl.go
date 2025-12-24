package main

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/rexlx/squall/internal"
	pb "github.com/rexlx/squall/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GrpcServer struct {
	pb.UnimplementedChatServiceServer
	appServer *Server
	streams   map[string]map[string]pb.ChatService_StreamServer
	streamMu  sync.RWMutex
}

func NewGrpcServer(app *Server) *GrpcServer {
	return &GrpcServer{
		appServer: app,
		streams:   make(map[string]map[string]pb.ChatService_StreamServer),
	}
}

// Login implementation...
func (s *GrpcServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	user, err := s.appServer.DB.GetUserByEmail(req.Email)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	ok, err := user.PasswordMatches(req.Password)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal auth error")
	}
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	if err := s.appServer.DB.StoreUser(user); err != nil {
		return nil, status.Error(codes.Internal, "failed to update user session")
	}

	s.appServer.Memory.Lock()
	s.appServer.Stats["logins"] = append(s.appServer.Stats["logins"], internal.Stat{Value: 1})
	s.appServer.Memory.Unlock()

	token, err := GenerateJWT(user.ID, s.appServer.Key)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	return &pb.LoginResponse{
		User: &pb.User{
			Id:        user.ID,
			Email:     user.Email,
			FirstName: user.Name,
			Rooms:     user.Rooms,
			History:   user.History,
		},
		Token: token,
	}, nil
}

// JoinRoom with Deduplication Logic
func (s *GrpcServer) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.RoomResponse, error) {
	roomName := req.RoomName
	room, err := s.appServer.DB.GetRoom(roomName)

	if err != nil {
		newRoom := Room{
			ID:          roomName,
			Name:        roomName,
			MaxMessages: 1000,
		}
		if err := s.appServer.DB.StoreRoom(newRoom); err != nil {
			return nil, status.Error(codes.Internal, "failed to create new room")
		}
		room = newRoom
	}

	var history []*pb.ChatMessage
	for _, m := range room.Messages {
		history = append(history, ToProto(m))
	}

	// Update User
	user, err := s.appServer.DB.GetUserByEmail(req.Email)
	if err == nil {
		// 1. Deduplicate History: Filter out existing occurrence of this room
		var cleanHistory []string
		for _, h := range user.History {
			if h != roomName {
				cleanHistory = append(cleanHistory, h)
			}
		}
		// Append to the end (Most Recent)
		user.History = append(cleanHistory, roomName)

		// 2. Add to Saved Rooms (Unique)
		exists := false
		for _, r := range user.Rooms {
			if r == roomName {
				exists = true
				break
			}
		}
		if !exists {
			user.Rooms = append(user.Rooms, roomName)
		}

		if err := s.appServer.DB.StoreUser(user); err != nil {
			fmt.Printf("Failed to save user updates: %v\n", err)
		}
	}

	return &pb.RoomResponse{
		RoomId:  room.ID,
		Name:    room.Name,
		Success: true,
		History: history,
	}, nil
}

// Stream implementation...
func (s *GrpcServer) Stream(stream pb.ChatService_StreamServer) error {
	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}

	userID := firstMsg.UserId
	roomID := firstMsg.RoomId

	s.registerStream(roomID, userID, stream)
	defer s.deregisterStream(roomID, userID)

	if firstMsg.MessageContent != "" {
		internalMsg := FromProto(firstMsg)
		if err := s.appServer.DB.StoreMessage(roomID, internalMsg); err != nil {
			fmt.Printf("Error saving first message: %v\n", err)
		}
		s.Broadcast(firstMsg)
	}

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		internalMsg := FromProto(msg)
		if err := s.appServer.DB.StoreMessage(msg.RoomId, internalMsg); err != nil {
			fmt.Printf("Error saving message: %v\n", err)
		}

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
		stream.Send(msg)
	}
}
