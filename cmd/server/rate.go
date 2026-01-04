package main

import (
	"context"
	"net"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// visitor wraps the rate limiter with a timestamp for TTL pruning
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter manages rate limits per IP address with automated cleanup
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	r        rate.Limit // Request limit (requests/sec)
	b        int        // Burst limit
}

// NewRateLimiter initializes the limiter and starts the background cleanup goroutine
func NewRateLimiter(rps int, burst int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		r:        rate.Limit(rps),
		b:        burst,
	}

	// Start background cleanup to prevent memory exhaustion
	go rl.cleanupVisitors()

	return rl
}

// getLimiter returns (or creates) the limiter for a specific IP and updates its TTL
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		v = &visitor{
			limiter: rate.NewLimiter(rl.r, rl.b),
		}
		rl.visitors[ip] = v
	}

	// Update the last seen time every time the limiter is accessed
	v.lastSeen = time.Now()

	return v.limiter
}

// cleanupVisitors periodically removes IPs that haven't been seen in over 3 minutes
func (rl *RateLimiter) cleanupVisitors() {
	for {
		time.Sleep(1 * time.Minute)

		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > 3*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// UnaryInterceptor protects unary calls (Login, CreateUser, etc.)
func (rl *RateLimiter) UnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ip := rl.extractIP(ctx)

	if !rl.getLimiter(ip).Allow() {
		return nil, status.Errorf(codes.ResourceExhausted, "too many requests - slow down")
	}

	return handler(ctx, req)
}

// StreamInterceptor for stream limits (like Connect/Stream)
func (rl *RateLimiter) StreamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ip := rl.extractIP(ss.Context())

	if !rl.getLimiter(ip).Allow() {
		return status.Errorf(codes.ResourceExhausted, "too many requests - slow down")
	}

	return handler(srv, ss)
}

// extractIP helper to get the remote IP from gRPC context
func (rl *RateLimiter) extractIP(ctx context.Context) string {
	if p, ok := peer.FromContext(ctx); ok {
		if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
			return host
		}
		return p.Addr.String()
	}
	return "unknown"
}
