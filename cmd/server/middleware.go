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
