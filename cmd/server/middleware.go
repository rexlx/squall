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

// AuthInterceptor checks for a valid JWT and injects a lightweight User into the context.
// It uses a stateless strategy, relying on claims within the token to avoid database bottlenecks.
func (s *GrpcServer) AuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// 1. Skip Auth for Login
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

	token := strings.TrimPrefix(values[0], "Bearer ")

	// 3. Validate Token
	claims, err := ValidateJWT(token, s.appServer.Key)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "access token is invalid: "+err.Error())
	}

	// 4. Populate lightweight User from Claims (Stateless Strategy)
	// Prerequisite: UserClaims must be updated to include Role and Email.
	user := User{
		ID:    claims.UserID,
		Role:  claims.Role,
		Email: claims.Email,
	}

	// 5. Inject User into Context
	newCtx := context.WithValue(ctx, userContextKey, user)

	return handler(newCtx, req)
}

// GetUserFromContext retrieves the user injected by the interceptors.
func GetUserFromContext(ctx context.Context) (User, error) {
	user, ok := ctx.Value(userContextKey).(User)
	if !ok {
		return User{}, status.Error(codes.Internal, "user not found in context")
	}
	return user, nil
}

// StreamAuthInterceptor handles authentication for streaming RPCs (like Chat).
func (s *GrpcServer) StreamAuthInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx := ss.Context()

	// 1. Extract Token from Metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "metadata is not provided")
	}

	values := md["authorization"]
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "authorization token is not provided")
	}

	token := strings.TrimPrefix(values[0], "Bearer ")

	// 2. Validate Token
	claims, err := ValidateJWT(token, s.appServer.Key)
	if err != nil {
		return status.Error(codes.Unauthenticated, "access token is invalid")
	}

	// 3. Populate lightweight User from Claims (Stateless Strategy)
	user := User{
		ID:    claims.UserID,
		Role:  claims.Role,
		Email: claims.Email,
	}

	// 4. Inject User into Context via WrappedServerStream
	newCtx := context.WithValue(ctx, userContextKey, user)
	wrappedStream := &WrappedServerStream{
		ServerStream: ss,
		ctx:          newCtx,
	}

	return handler(srv, wrappedStream)
}

// WrappedServerStream allows overriding the Context of a gRPC stream.
type WrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *WrappedServerStream) Context() context.Context {
	return w.ctx
}
