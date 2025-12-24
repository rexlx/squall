// File: cmd/admin-cli/main.go
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
	// Flags for the Admin (the person running the tool)
	adminEmail := flag.String("admin", "", "Existing Admin email")
	adminPass := flag.String("pass", "", "Existing Admin password")

	// Flags for the New User (the account being created)
	newEmail := flag.String("new-email", "", "New user email")
	newPass := flag.String("new-pass", "", "New user password")
	newName := flag.String("new-name", "", "New user name")
	newRole := flag.String("new-role", "user", "New user role (user/admin)")

	host := flag.String("host", "localhost:8080", "Server host:port")

	flag.Parse()

	if *adminEmail == "" || *newEmail == "" {
		log.Fatal("Usage: go run cmd/admin-cli/main.go -admin <email> -pass <pass> -new-email <email> -new-pass <pass> ...")
	}

	// 1. Connect to Server
	// InsecureSkipVerify is used because we are likely using self-signed certs locally
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	conn, err := grpc.Dial(*host, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewChatServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 2. Login as Admin to get Token
	fmt.Printf("Authenticating as %s...\n", *adminEmail)
	loginResp, err := client.Login(ctx, &pb.LoginRequest{
		Email:    *adminEmail,
		Password: *adminPass,
	})
	if err != nil {
		log.Fatalf("Login failed: %v", err)
	}
	if loginResp.Error {
		log.Fatalf("Login error: %s", loginResp.Message)
	}
	fmt.Println("Authentication successful.")

	// 3. Create the New User
	// We attach the token to the context metadata so the Middleware can see it
	md := metadata.Pairs("authorization", loginResp.Token)
	authCtx := metadata.NewOutgoingContext(ctx, md)

	createResp, err := client.CreateUser(authCtx, &pb.CreateUserRequest{
		Email:     *newEmail,
		Password:  *newPass,
		FirstName: *newName,
		Role:      *newRole,
	})

	if err != nil {
		log.Fatalf("CreateUser failed: %v", err)
	}

	if createResp.Success {
		fmt.Printf("\n[SUCCESS] Created User:\n  ID: %s\n  Email: %s\n  Role: %s\n", createResp.UserId, *newEmail, *newRole)
	} else {
		fmt.Printf("\n[FAILURE] Server Message: %s\n", createResp.Message)
	}
}
