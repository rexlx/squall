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

func (s *GrpcServer) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	// 1. Authorization Check (via Middleware)
	caller, err := GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if caller.Role != "admin" {
		return nil, status.Error(codes.PermissionDenied, "permission denied: only admins can create users")
	}

	// 2. Validate Input
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	// Check if user already exists
	existingUser, err := s.appServer.DB.GetUserByEmail(req.Email)
	if err == nil && existingUser.ID != "" {
		return nil, status.Error(codes.AlreadyExists, "user with this email already exists")
	}

	// 3. Create User Object
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	newID := hex.EncodeToString(randBytes)

	// Default to "user" unless specifically set to "admin"
	newRole := "user"
	if req.Role == "admin" {
		newRole = "admin"
	}

	newUser := User{
		ID:      newID,
		Email:   req.Email,
		Name:    req.FirstName,
		Role:    newRole,
		Created: time.Now(),
		Updated: time.Now(),
	}

	// Hash Password
	if err := newUser.SetPassword(req.Password); err != nil {
		return nil, status.Error(codes.Internal, "failed to hash password")
	}

	// 4. Store in DB
	if err := s.appServer.DB.StoreUser(newUser); err != nil {
		return nil, status.Error(codes.Internal, "failed to store user: "+err.Error())
	}

	s.appServer.Logger.Printf("ADMIN ACTION: %s created new user %s (%s)", caller.Email, newUser.Email, newUser.Role)

	return &pb.CreateUserResponse{
		Success: true,
		UserId:  newUser.ID,
		Message: "User created successfully",
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
// func (s *GrpcServer) Stream(stream pb.ChatService_StreamServer) error {
// 	firstMsg, err := stream.Recv()
// 	if err != nil {
// 		return err
// 	}

// 	userID := firstMsg.UserId
// 	roomID := firstMsg.RoomId

// 	s.registerStream(roomID, userID, stream)
// 	defer s.deregisterStream(roomID, userID)

// 	if firstMsg.MessageContent != "" {
// 		internalMsg := FromProto(firstMsg)
// 		if err := s.appServer.DB.StoreMessage(roomID, internalMsg); err != nil {
// 			fmt.Printf("Error saving first message: %v\n", err)
// 		}
// 		s.Broadcast(firstMsg)
// 	}

// 	for {
// 		msg, err := stream.Recv()
// 		if err == io.EOF {
// 			return nil
// 		}
// 		if err != nil {
// 			return err
// 		}

// 		internalMsg := FromProto(msg)
// 		if err := s.appServer.DB.StoreMessage(msg.RoomId, internalMsg); err != nil {
// 			fmt.Printf("Error saving message: %v\n", err)
// 		}

// 		s.Broadcast(msg)
// 	}
// }

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

// Stream implementation with Async Save and Non-blocking Broadcast
func (s *GrpcServer) Stream(stream pb.ChatService_StreamServer) error {
	// 1. Authenticate via Context (injected by Middleware)
	user, err := GetUserFromContext(stream.Context())
	if err != nil {
		return err
	}

	// 2. Wait for first message to establish Room
	// (Your protocol sends the room ID in the first message)
	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}

	roomID := firstMsg.RoomId
	userID := user.ID

	// 3. Register Stream
	s.registerStream(roomID, userID, stream)
	s.appServer.Logger.Printf("Stream connected: %s @ %s", user.Email, roomID)

	defer func() {
		s.deregisterStream(roomID, userID)
		s.appServer.Logger.Printf("Stream disconnected: %s", user.Email)
	}()

	// Handle the first message immediately
	if firstMsg.MessageContent != "" {
		s.processMessage(user, firstMsg)
	}

	// 4. Receive Loop
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

// Helper to handle message processing (Async Save + Broadcast)
func (s *GrpcServer) processMessage(user User, msg *pb.ChatMessage) {
	// Construct internal message using YOUR specific fields
	internalMsg := internal.Message{
		RoomID:        msg.RoomId,
		UserID:        user.ID,
		Email:         user.Email,
		Message:       msg.MessageContent,                   // Maps to 'Content' in proto
		InitialVector: msg.Iv,                               // Maps to 'Iv'
		HotSauce:      msg.HotSauce,                         // Maps to 'HotSauce'
		Time:          fmt.Sprintf("%d", time.Now().Unix()), // time_str string
	}

	// 1. ASYNC SAVE: Push to queue
	// This uses the MsgQueue we added to AppServer in step 2.
	// If the queue is full, we skip saving to preserve chat responsiveness.
	select {
	case s.appServer.Queue <- SaveRequest{RoomID: msg.RoomId, Message: internalMsg}:
	default:
		s.appServer.Logger.Println("WARNING: DB Queue full, dropping message persistence.")
	}

	// 2. BROADCAST
	// Update timestamp on the outgoing message to match server time
	msg.Timestamp = time.Now().Unix()
	s.Broadcast(msg)
}

// Broadcast sends a message to all users IN THE SPECIFIC ROOM.
func (s *GrpcServer) Broadcast(msg *pb.ChatMessage) {
	// 1. Snapshot valid streams for this room
	s.streamMu.RLock()
	roomStreams, exists := s.streams[msg.RoomId]

	// If room has no streams, unlock and return
	if !exists || len(roomStreams) == 0 {
		s.streamMu.RUnlock()
		return
	}

	// Create a slice to hold the streams we need to write to
	activeStreams := make([]pb.ChatService_StreamServer, 0, len(roomStreams))
	for _, stream := range roomStreams {
		activeStreams = append(activeStreams, stream)
	}
	s.streamMu.RUnlock()
	// LOCK RELEASED HERE - New users can now join/login without waiting for this broadcast.

	// 2. Send to clients
	for _, stream := range activeStreams {
		// Ignore errors (broken pipe, etc); they are handled in the Stream loop
		_ = stream.Send(msg)
	}
}
