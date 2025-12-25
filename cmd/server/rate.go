package main

import (
	"context"
	"net"
	"sync"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// RateLimiter manages rate limits per IP address
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*rate.Limiter
	r        rate.Limit // Request limit (e.g., 5 requests/sec)
	b        int        // Burst limit (e.g., allow 10 at once)
}

func NewRateLimiter(rps int, burst int) *RateLimiter {
	return &RateLimiter{
		visitors: make(map[string]*rate.Limiter),
		r:        rate.Limit(rps),
		b:        burst,
	}
}

// getLimiter returns (or creates) the limiter for a specific IP
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.visitors[ip]
	if !exists {
		limiter = rate.NewLimiter(rl.r, rl.b)
		rl.visitors[ip] = limiter
	}

	return limiter
}

// Interceptor is the gRPC middleware function
func (rl *RateLimiter) Interceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// 1. Extract IP from context
	ip := "unknown"
	if p, ok := peer.FromContext(ctx); ok {
		// p.Addr.String() returns "IP:Port", we just want IP
		if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
			ip = host
		}
	}

	// 2. Check Limit
	limiter := rl.getLimiter(ip)
	if !limiter.Allow() {
		return nil, status.Errorf(codes.ResourceExhausted, "too many requests - slow down")
	}

	// 3. Cleanup (Optional: Simple map cleanup to prevent memory leaks)
	// In a real prod app, use an LRU cache or run a background cleanup goroutine.

	return handler(ctx, req)
}

// UnaryInterceptor protects unary calls (Login, CreateUser, etc.)
func (rl *RateLimiter) UnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ip := "unknown"
	if p, ok := peer.FromContext(ctx); ok {
		// p.Addr.String() typically returns "IP:Port"
		if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
			ip = host
		}
	}

	if !rl.getLimiter(ip).Allow() {
		return nil, status.Errorf(codes.ResourceExhausted, "too many requests - slow down")
	}

	return handler(ctx, req)
}

// StreamInterceptor for stream limits (like Connect/Stream)
func (rl *RateLimiter) StreamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ip := "unknown"
	if p, ok := peer.FromContext(ss.Context()); ok {
		if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
			ip = host
		}
	}

	if !rl.getLimiter(ip).Allow() {
		return status.Errorf(codes.ResourceExhausted, "too many requests - slow down")
	}

	return handler(srv, ss)
}
