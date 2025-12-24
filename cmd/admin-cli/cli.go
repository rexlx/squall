package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"time"

	pb "github.com/rexlx/squall/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

func main() {
	// CLI Flags
	adminEmail := flag.String("admin", "", "Existing Admin email")
	adminPass := flag.String("pass", "", "Existing Admin password")
	newEmail := flag.String("new-email", "", "New user email")
	newPass := flag.String("new-pass", "", "New user password")
	newName := flag.String("new-name", "", "New user name")
	newRole := flag.String("new-role", "user", "New user role (user|admin)")
	host := flag.String("host", "localhost:8080", "Server host:port")

	flag.Parse()

	if *adminEmail == "" || *adminPass == "" || *newEmail == "" || *newPass == "" {
		log.Fatal("Usage: go run cmd/admin-cli/main.go -admin <email> -pass <pass> -new-email <target> -new-pass <pass> ...")
	}

	// 1. Load Client Certificates (mTLS)
	// We must present a certificate to the server, or it will reject the connection.
	cert, err := tls.LoadX509KeyPair("data/client-cert.pem", "data/client-key.pem")
	if err != nil {
		log.Fatalf("Failed to load client certs: %v", err)
	}

	// 2. Configure TLS
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true, // Still needed for self-signed server certs
	}
	creds := credentials.NewTLS(tlsConfig)

	// 3. Connect to Server
	conn, err := grpc.Dial(*host, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewChatServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 4. Login as Admin
	fmt.Printf("Logging in as %s...\n", *adminEmail)
	loginResp, err := client.Login(ctx, &pb.LoginRequest{
		Email:    *adminEmail,
		Password: *adminPass,
	})
	if err != nil {
		log.Fatalf("Login RPC failed: %v", err)
	}
	if loginResp.Error {
		log.Fatalf("Login refused: %s", loginResp.Message)
	}

	fmt.Println("Login successful. Creating new user...")

	// 5. Create New User
	// Attach token to context
	md := metadata.Pairs("authorization", loginResp.Token)
	authCtx := metadata.NewOutgoingContext(ctx, md)

	createResp, err := client.CreateUser(authCtx, &pb.CreateUserRequest{
		Email:     *newEmail,
		Password:  *newPass,
		FirstName: *newName,
		Role:      *newRole,
	})
	if err != nil {
		log.Fatalf("CreateUser RPC failed: %v", err)
	}

	if createResp.Success {
		fmt.Printf("SUCCESS: User '%s' created (ID: %s)\n", *newEmail, createResp.UserId)
	} else {
		fmt.Printf("FAILED: Server returned error: %s\n", createResp.Message)
	}
}
