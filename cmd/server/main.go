package main

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/rexlx/squall/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// --- MAIN SERVER LOGIC ---

func main() {
	// 1. Parse Flags
	firstUse := flag.Bool("firstuse", false, "Initialize the server by creating the first admin user")
	// Note: We removed the prune-freq flag for this production-ready file,
	// but you can add it back if you kept the worker logic from the benchmark discussion.
	flag.Parse()

	// 2. Setup Logging
	// For containerized/public deploys, logging to Stdout is preferred over a file
	logger := log.New(os.Stdout, "SERVER: ", log.LstdFlags|log.Lshortfile)

	// 3. Load Secrets from Environment (Security Fix)
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		// Fallback for local dev convenience, but warn heavily
		logger.Println("WARNING: DB_DSN not set, using default insecure local DSN")
		dsn = "user=rxlx password=thereISnosp0)n host=localhost dbname=chaps sslmode=disable"
	}

	jwtKey := os.Getenv("JWT_SECRET")
	if jwtKey == "" {
		logger.Fatal("CRITICAL: JWT_SECRET environment variable must be set.")
	}
	WhitelistMu.Lock()
	Whitelist["test@example.com"] = true
	WhitelistMu.Unlock()

	// 4. Connect to Database
	db, err := NewPostgresDB(dsn)
	if err != nil {
		logger.Fatal("Failed to connect to database:", err)
	}
	if err = db.CreateTables(); err != nil {
		logger.Fatal("Failed to create tables:", err)
	}
	logger.Println("Database connected.")

	// 5. Handle First Use
	if *firstUse {
		createFirstUser(db)
		os.Exit(0)
	}

	// 6. Initialize Application Logic
	appServer := NewServer("0.0.0.0:8080", jwtKey, logger, db)
	// Start the SaveWorker (assuming you kept the simplified worker from previous discussions)
	go appServer.StartSaveWorker()
	go appServer.StartPruneWorker(1*time.Hour, 1000)
	go appServer.StartRoomReaper(6*time.Hour, 49*time.Hour)
	grpcImpl := NewGrpcServer(appServer)

	// 7. Initialize Rate Limiter
	// Allow 5 requests per second, with a burst of 10
	limiter := NewRateLimiter(5, 10)

	// 8. Configure gRPC Options (TLS vs No-TLS)
	var opts []grpc.ServerOption

	if os.Getenv("DISABLE_TLS") == "true" {
		logger.Println("Running in NO-TLS mode (SSL Termination expected upstream)")
		// No credentials added, server runs in h2c/plaintext mode
	} else {
		logger.Println("Running in TLS mode")
		// Load certs for standard HTTPS (No mTLS)
		// Ensure these files exist in your container/server
		tlsConfig, err := loadServerTLSConfig("data/server-cert.pem", "data/server-key.pem")
		if err != nil {
			logger.Fatal("Failed to load TLS keys:", err)
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.Creds(creds))
	}

	// 9. Chain Interceptors (Rate Limit -> Auth)
	opts = append(opts,
		grpc.ChainUnaryInterceptor(
			limiter.UnaryInterceptor, // 1. Check Rate Limit
			grpcImpl.AuthInterceptor, // 2. Check Auth Token
		),
		grpc.ChainStreamInterceptor(
			limiter.StreamInterceptor,      // 1. Check Rate Limit
			grpcImpl.StreamAuthInterceptor, // 2. Check Auth Token
		),
	)

	// 10. Setup Listener
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Fatal("Failed to listen:", err)
	}
	logger.Printf("Server listening on port %s", port)

	// 11. Start Server
	grpcServer := grpc.NewServer(opts...)
	proto.RegisterChatServiceServer(grpcServer, grpcImpl)

	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("Failed to serve gRPC:", err)
	}
}

// --- HELPER FUNCTIONS ---

func createFirstUser(db Database) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("--- FIRST USE SETUP (Creating ADMIN User) ---")

	fmt.Print("Enter Admin Email: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)

	fmt.Print("Enter Admin Password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	fmt.Print("Enter Admin Name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)

	if email == "" || password == "" {
		fmt.Println("Error: Email and Password are required.")
		os.Exit(1)
	}

	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	id := hex.EncodeToString(randBytes)

	newUser := User{
		ID:      id,
		Email:   email,
		Name:    name,
		Role:    "admin",
		Created: time.Now(),
		Updated: time.Now(),
	}

	if err := newUser.SetPassword(password); err != nil {
		fmt.Printf("Error hashing password: %v\n", err)
		os.Exit(1)
	}

	if err := db.StoreUser(newUser); err != nil {
		fmt.Printf("Error storing user: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully created ADMIN user:", email)
	fmt.Println("Setup complete. Restart server without -firstuse flag.")
}

// loadServerTLSConfig loads keys for standard HTTPS (Server-Side TLS only)
func loadServerTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	serverCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		// Removed ClientCAs and ClientAuth to allow public connections
		// This makes it a standard HTTPS/TLS server
	}

	return config, nil
}
