package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/rexlx/squall/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

// Config
var (
	host       = flag.String("host", "neo.nullferatu.com:8085", "Server address")
	adminEmail = flag.String("admin", "rex@aol.com", "Admin email")
	adminPass  = flag.String("pass", "admin", "Admin password")

	// Benchmark control
	numUsers = flag.Int("users", 50, "Concurrent users")
	numRooms = flag.Int("rooms", 10, "Rooms per user")
	msgRate  = flag.Int("rate", 1000, "Interval (ms) between messages per user")

	// Feature flags
	ensurePrune = flag.Bool("prune-heavy", false, "Overrides rates/users to GUARANTEE hitting prune limits")
)

// Stats Collection
type Stats struct {
	Sent     uint64
	Recv     uint64
	Errors   uint64
	TotalLat int64 // Microseconds
	MaxLat   int64 // Microseconds
}

var globalStats Stats

func main() {
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	if *ensurePrune {
		log.Println("!!! PRUNE HEAVY MODE ENABLED !!!")
		*numUsers = 50
		*msgRate = 100
		log.Printf("Adjusted configuration: %d Users @ %dms interval", *numUsers, *msgRate)
	}

	cert, err := tls.LoadX509KeyPair("data/client-cert.pem", "data/client-key.pem")
	if err != nil {
		log.Fatalf("Cert load failed: %v", err)
	}
	creds := credentials.NewTLS(&tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	})

	token := setupEnv(creds)

	go runReporter()

	log.Printf("Launching %d bots...", *numUsers)
	var wg sync.WaitGroup
	wg.Add(*numUsers)

	for i := 0; i < *numUsers; i++ {
		go func(id int) {
			defer wg.Done()
			runBot(id, creds, token)
		}(i)
		time.Sleep(20 * time.Millisecond)
	}
	wg.Wait()
}

func runReporter() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		sent := atomic.SwapUint64(&globalStats.Sent, 0)
		recv := atomic.SwapUint64(&globalStats.Recv, 0)
		totLat := atomic.SwapInt64(&globalStats.TotalLat, 0)
		maxLat := atomic.SwapInt64(&globalStats.MaxLat, 0)

		var avgLat float64
		if sent > 0 {
			avgLat = float64(totLat) / float64(sent) / 1000.0
		}
		maxLatMs := float64(maxLat) / 1000.0

		log.Printf("STATS [1s]: Sent: %d | Recv: %d | Latency: Avg %.2fms / Max %.2fms",
			sent, recv, avgLat, maxLatMs)
	}
}

func setupEnv(creds credentials.TransportCredentials) string {
	conn, err := grpc.Dial(*host, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()
	client := pb.NewChatServiceClient(conn)

	resp, err := client.Login(context.Background(), &pb.LoginRequest{
		Email: *adminEmail, Password: *adminPass,
	})
	if err != nil {
		log.Fatalf("Admin login failed: %v", err)
	}

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", resp.Token))
	for i := 0; i < *numUsers; i++ {
		// Ignore errors here in case users already exist
		_, _ = client.CreateUser(ctx, &pb.CreateUserRequest{
			Email:     fmt.Sprintf("bench_%d@test.com", i),
			Password:  "password",
			FirstName: fmt.Sprintf("Bot-%d", i),
			Role:      "user",
		})
	}
	return resp.Token
}

func runBot(id int, creds credentials.TransportCredentials, adminToken string) {
	conn, err := grpc.Dial(*host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return
	}
	defer conn.Close()
	client := pb.NewChatServiceClient(conn)

	email := fmt.Sprintf("bench_%d@test.com", id)
	lResp, err := client.Login(context.Background(), &pb.LoginRequest{Email: email, Password: "password"})
	if err != nil || lResp.User == nil {
		log.Printf("Bot %d login failed", id)
		return
	}

	authCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", lResp.Token))

	for r := 0; r < *numRooms; r++ {
		roomName := fmt.Sprintf("stress_room_%d", r)
		client.JoinRoom(authCtx, &pb.JoinRoomRequest{Email: email, RoomName: roomName})
		go startStream(client, authCtx, lResp.User.Id, roomName)
	}
	select {}
}

func startStream(client pb.ChatServiceClient, ctx context.Context, userID, roomID string) {
	stream, err := client.Stream(ctx)
	if err != nil {
		return
	}

	// Handshake: Send empty message with RoomID/UserID to register stream
	stream.Send(&pb.ChatMessage{UserId: userID, RoomId: roomID})

	go func() {
		for {
			if _, err := stream.Recv(); err != nil {
				return
			}
			atomic.AddUint64(&globalStats.Recv, 1)
		}
	}()

	ticker := time.NewTicker(time.Duration(*msgRate) * time.Millisecond)
	time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)

	for range ticker.C {
		// FIXED: Use Payload wrapper for oneof and set Type explicitly
		msg := &pb.ChatMessage{
			UserId: userID,
			RoomId: roomID,
			Type:   pb.ChatMessage_TEXT,
			Payload: &pb.ChatMessage_MessageContent{
				MessageContent: "cGluZw==", // "ping"
			},
			Timestamp: time.Now().Unix(),
		}

		start := time.Now()
		err := stream.Send(msg)
		dur := time.Since(start).Microseconds()

		if err == nil {
			atomic.AddUint64(&globalStats.Sent, 1)
			atomic.AddInt64(&globalStats.TotalLat, dur)

			for {
				currMax := atomic.LoadInt64(&globalStats.MaxLat)
				if dur <= currMax {
					break
				}
				if atomic.CompareAndSwapInt64(&globalStats.MaxLat, currMax, dur) {
					break
				}
			}
		} else {
			atomic.AddUint64(&globalStats.Errors, 1)
			return
		}
	}
}
