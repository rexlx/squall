package main

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextKey string

const userContextKey contextKey = "user"

// AuthInterceptor is a middleware that checks for a valid JWT and injects the User into the context.
func (s *GrpcServer) AuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// 1. Skip Auth for Login (and other public endpoints if any)
	if info.FullMethod == "/chat.ChatService/Login" {
		return handler(ctx, req)
	}

	// 2. Extract Token from Metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "metadata is not provided")
	}

	values := md["authorization"]
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "authorization token is not provided")
	}

	// Support "Bearer <token>" or just "<token>"
	token := values[0]
	token = strings.TrimPrefix(token, "Bearer ")

	// 3. Validate Token
	claims, err := ValidateJWT(token, s.appServer.Key)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "access token is invalid: "+err.Error())
	}

	// 4. Fetch User from DB
	// This ensures the user still exists and we have their latest Role
	user, err := s.appServer.DB.GetUser(claims.UserID)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "user not found")
	}

	// 5. Inject User into Context
	newCtx := context.WithValue(ctx, userContextKey, user)

	return handler(newCtx, req)
}

// Helper to retrieve user from context in your handlers
func GetUserFromContext(ctx context.Context) (User, error) {
	user, ok := ctx.Value(userContextKey).(User)
	if !ok {
		return User{}, status.Error(codes.Internal, "user not found in context")
	}
	return user, nil
}

// StreamAuthInterceptor handles authentication for streaming RPCs (like Chat)
func (s *GrpcServer) StreamAuthInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	// 1. Get Context from the stream
	ctx := ss.Context()

	// 2. Extract Token from Metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "metadata is not provided")
	}

	values := md["authorization"]
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "authorization token is not provided")
	}

	token := strings.TrimPrefix(values[0], "Bearer ")

	// 3. Validate Token
	claims, err := ValidateJWT(token, s.appServer.Key)
	if err != nil {
		return status.Error(codes.Unauthenticated, "access token is invalid")
	}

	// 4. Fetch User from DB
	user, err := s.appServer.DB.GetUser(claims.UserID)
	if err != nil {
		return status.Error(codes.Unauthenticated, "user not found")
	}

	// 5. Inject User into Context
	// We must wrap the ServerStream to modify the Context it returns
	newCtx := context.WithValue(ctx, userContextKey, user)
	wrappedStream := &WrappedServerStream{
		ServerStream: ss,
		ctx:          newCtx,
	}

	return handler(srv, wrappedStream)
}

// Wrapper to override Context()
type WrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *WrappedServerStream) Context() context.Context {
	return w.ctx
}
