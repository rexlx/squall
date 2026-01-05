package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"

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

	token, err := GenerateJWT(user.ID, user.Role, user.Email, s.appServer.Key)
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

func (s *GrpcServer) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	caller, err := GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if caller.Role != "admin" {
		return nil, status.Error(codes.PermissionDenied, "only admins can create users")
	}

	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	newID := hex.EncodeToString(randBytes)

	newUser := User{
		ID:      newID,
		Email:   req.Email,
		Name:    req.FirstName,
		Role:    req.Role,
		Created: time.Now(),
		Updated: time.Now(),
	}

	if err := newUser.SetPassword(req.Password); err != nil {
		return nil, status.Error(codes.Internal, "failed to hash password")
	}

	if err := s.appServer.DB.StoreUser(newUser); err != nil {
		return nil, status.Error(codes.Internal, "failed to store user")
	}

	return &pb.CreateUserResponse{Success: true, UserId: newID}, nil
}

// cmd/server/grpc_impl.go updates

func (s *GrpcServer) UpdatePassword(ctx context.Context, req *pb.UpdatePasswordRequest) (*pb.UpdatePasswordResponse, error) {
	caller, err := GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Security: Users can only update their own password unless they are an admin
	if caller.Email != req.Email && caller.Role != "admin" {
		return nil, status.Error(codes.PermissionDenied, "unauthorized password update")
	}

	user, err := s.appServer.DB.GetUserByEmail(req.Email)
	if err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	// Verify old password (skip check if admin is performing the update)
	if caller.Role != "admin" {
		ok, _ := user.PasswordMatches(req.OldPassword)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "incorrect old password")
		}
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		return nil, status.Error(codes.Internal, "failed to hash password")
	}

	if err := s.appServer.DB.StoreUser(user); err != nil {
		return nil, status.Error(codes.Internal, "failed to update user")
	}

	return &pb.UpdatePasswordResponse{Success: true, Message: "Password updated successfully"}, nil
}

func (s *GrpcServer) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.RoomResponse, error) {
	roomName := req.RoomName
	room, err := s.appServer.DB.GetRoom(roomName)

	if err != nil {
		room = Room{ID: roomName, Name: roomName, MaxMessages: 1000}
		s.appServer.DB.StoreRoom(room)
	}

	// --- FIX: Persist History and Saved Rooms ---
	caller, err := GetUserFromContext(ctx)
	if err == nil {
		fmt.Println("no user in context:", err)
		dbUser, _ := s.appServer.DB.GetUserByEmail(caller.Email)

		// 1. Update Saved Rooms (Unique list)
		found := false
		for _, r := range dbUser.Rooms {
			if r == roomName {
				found = true
				break
			}
		}
		if !found {
			dbUser.Rooms = append(dbUser.Rooms, roomName)
		}

		// 2. Update History (Move current room to the front, limit to 10)
		newHistory := []string{roomName}
		for _, r := range dbUser.History {
			if r != roomName {
				newHistory = append(newHistory, r)
			}
		}
		if len(newHistory) > 10 {
			newHistory = newHistory[:10]
		}
		dbUser.History = newHistory

		// Persist changes to database
		s.appServer.DB.StoreUser(dbUser)
	}

	var history []*pb.ChatMessage
	for _, m := range room.Messages {
		history = append(history, ToProto(m))
	}

	return &pb.RoomResponse{
		RoomId:  room.ID,
		Name:    room.Name,
		Success: true,
		History: history,
	}, nil
}

func (s *GrpcServer) Stream(stream pb.ChatService_StreamServer) error {
	user, err := GetUserFromContext(stream.Context())
	if err != nil {
		return err
	}

	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}

	roomID := firstMsg.RoomId
	userID := user.ID

	s.registerStream(roomID, userID, stream)
	defer s.deregisterStream(roomID, userID)

	// Use GetMessageContent() accessor for the oneof field
	if firstMsg.GetMessageContent() != "" {
		s.processMessage(user, firstMsg)
	}

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		s.processMessage(user, msg)
	}
}

func (s *GrpcServer) processMessage(user User, msg *pb.ChatMessage) {
	msg.Timestamp = time.Now().Unix()
	s.Broadcast(msg)

	// Don't save binary chunks to the DB
	if msg.Type == pb.ChatMessage_FILE_CHUNK {
		return
	}

	var dbContent string
	switch msg.Type {
	case pb.ChatMessage_TEXT:
		dbContent = msg.GetMessageContent()
	case pb.ChatMessage_FILE_CONTROL:
		if meta := msg.GetFileMeta(); meta != nil {
			dbContent = fmt.Sprintf("FILE:%s|HASH:%s|ACTION:%s", meta.FileName, meta.FileHash, meta.Action)
		}
	}

	internalMsg := internal.Message{
		RoomID:        msg.RoomId,
		UserID:        user.ID,
		Email:         user.Email,
		Message:       dbContent,
		InitialVector: msg.Iv,
		HotSauce:      msg.HotSauce,
		Time:          fmt.Sprintf("%d", msg.Timestamp),
	}

	select {
	case s.appServer.Queue <- SaveRequest{RoomID: msg.RoomId, Message: internalMsg}:
	default:
		s.appServer.Logger.Println("DB Queue full, dropping persistence.")
	}
}

func (s *GrpcServer) Broadcast(msg *pb.ChatMessage) {
	s.streamMu.RLock()
	roomStreams, exists := s.streams[msg.RoomId]
	if !exists || len(roomStreams) == 0 {
		s.streamMu.RUnlock()
		return
	}

	activeStreams := make([]pb.ChatService_StreamServer, 0, len(roomStreams))
	for _, stream := range roomStreams {
		activeStreams = append(activeStreams, stream)
	}
	s.streamMu.RUnlock()

	for _, stream := range activeStreams {
		_ = stream.Send(msg)
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
