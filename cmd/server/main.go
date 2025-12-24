package main

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/rexlx/squall/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// DSN for Postgres connection
var dsn = "user=rxlx password=thereISnosp0)n host=192.168.86.120 dbname=chaps sslmode=disable"

func main() {
	// Define flags
	firstUse := flag.Bool("firstuse", false, "Initialize the server by creating the first admin user")
	flag.Parse()

	// 1. Setup Logging
	file, err := os.OpenFile("thisserver.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("Error opening log file:", err)
		os.Exit(1)
	}
	defer file.Close()
	logger := log.New(file, "SERVER: ", log.LstdFlags|log.Lshortfile)

	// 2. Connect to Database
	db, err := NewPostgresDB(dsn)
	if err != nil {
		logger.Fatal("Failed to connect to database:", err)
	}

	// Ensure Tables Exist
	if err = db.CreateTables(); err != nil {
		logger.Fatal("Failed to create tables:", err)
	}
	logger.Println("Database connected and tables verified.")

	// 3. Handle First Use Flag
	if *firstUse {
		createFirstUser(db)
		// We exit after firstuse setup to ensure clean state,
		// remove this if you want to continue starting the server immediately.
		os.Exit(0)
	}

	// 4. Initialize Application Logic
	appServer := NewServer("0.0.0.0:8080", "system-key", logger, db)

	// 5. Initialize gRPC Implementation
	grpcImpl := NewGrpcServer(appServer)

	// 6. Load TLS Credentials (mTLS)
	tlsConfig, err := loadServerTLSConfig("data/server-cert.pem", "data/server-key.pem", "data/ca-cert.pem")
	if err != nil {
		logger.Fatal("Failed to load TLS keys:", err)
	}
	creds := credentials.NewTLS(tlsConfig)

	// 7. Setup Listener
	lis, err := net.Listen("tcp", ":8080")
	if err != nil {
		logger.Fatal("Failed to listen:", err)
	}
	logger.Printf("Server listening on %s (TLS Enabled)", ":8080")

	// 8. Create and Start gRPC Server
	grpcServer := grpc.NewServer(
		grpc.Creds(creds),
		grpc.UnaryInterceptor(grpcImpl.AuthInterceptor),
	)

	proto.RegisterChatServiceServer(grpcServer, grpcImpl)

	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("Failed to serve gRPC:", err)
	}
}

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

func loadServerTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	serverCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	caCert, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA cert")
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}

	return config, nil
}
