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

// Login matches the signature defined in your proto file
func (s *GrpcServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	// 1. Validation (Replaces checking r.Body or empty fields)
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	// 2. User Lookup (Business logic logic remains the same)
	// Note: You'll need to ensure your DB has a method to find by email,
	// as your KV store might only be keyed by ID.
	user, err := s.appServer.DB.GetUserByEmail(req.Email)
	if err != nil {
		// Differentiate between "System Error" and "Not Found" to avoid leaking user existence
		// or just return Unauthenticated for everything to be safe.
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	fmt.Println("User found:", user.Email)
	// 3. Password Check (Replaces u.PasswordMatches)
	ok, err := user.PasswordMatches(req.Password)
	if err != nil {
		// Internal hashing error
		return nil, status.Error(codes.Internal, "internal auth error")
	}
	if !ok {
		// HTTP: w.Write({"message": "password wrong"})
		// gRPC: Return specific error code
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	// 4. Update State (Replaces u.updateHandle and s.AddUser)
	// Assuming these modify the 'user' struct
	// user.updateHandle() // If you have this method
	if err := s.appServer.DB.StoreUser(user); err != nil {
		return nil, status.Error(codes.Internal, "failed to update user session")
	}

	// 5. Statistics (Replaces s.Stats.App["logins"]++)
	s.appServer.Memory.Lock()
	s.appServer.Stats["logins"] = append(s.appServer.Stats["logins"], internal.Stat{Value: 1})
	s.appServer.Memory.Unlock()

	// 6. Generate Token (Best Practice)
	// In gRPC, we rarely just return the User object. We return a Token
	// (JWT) that the client uses for future requests.
	token, err := GenerateJWT(user.ID, s.appServer.Key) // You would implement this helper
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	// 7. Success Response
	return &pb.LoginResponse{
		User: &pb.User{
			Id:        user.ID,
			Email:     user.Email,
			FirstName: user.Name,
			// Do NOT map the Password field back to the client
		},
		Token: token,
	}, nil
}

// 2. JoinRoom Implementation
func (s *GrpcServer) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.RoomResponse, error) {
	// Logic to find/create room in s.appServer.Rooms
	s.appServer.Memory.Lock()
	defer s.appServer.Memory.Unlock()

	// simplified logic
	roomID := "room-" + req.RoomName
	fmt.Println("new room", roomID)
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
