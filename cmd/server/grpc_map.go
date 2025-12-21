package main

import (
	"encoding/base64"
	"time"

	"github.com/rexlx/squall/internal"
	pb "github.com/rexlx/squall/proto" // Assuming you generate code here
)

// ToProto converts your internal/pure Go Message struct to the gRPC Proto struct
func ToProto(m internal.Message) *pb.ChatMessage {
	// 1. Handle Time conversion (String ISO8601 -> Int64 Unix)
	var ts int64
	parsedTime, err := time.Parse(time.RFC3339, m.Time)
	if err == nil {
		ts = parsedTime.Unix()
	} else {
		ts = time.Now().Unix() // Fallback
	}

	// 2. Handle Content (Base64 String -> Bytes)
	// Your client sends ciphertext as a base64 string. gRPC prefers raw bytes.
	decodedContent, _ := base64.StdEncoding.DecodeString(m.Message)

	return &pb.ChatMessage{
		RoomId:           m.RoomID,
		UserId:           m.UserID,
		EncryptedContent: decodedContent,
		Timestamp:        ts,
		ReplyTo:          m.ReplyTo,
		Email:            m.Email,
		Iv:               m.InitialVector,
		HotSauce:         m.HotSauce,
	}
}

// FromProto converts the gRPC Proto struct back to your internal/pure Go Message
func FromProto(p *pb.ChatMessage) internal.Message {
	// 1. Handle Time (Int64 Unix -> String ISO8601)
	t := time.Unix(p.Timestamp, 0).Format(time.RFC3339)

	// 2. Handle Content (Bytes -> Base64 String)
	encodedContent := base64.StdEncoding.EncodeToString(p.EncryptedContent)

	return internal.Message{
		RoomID:        p.RoomId,
		UserID:        p.UserId,
		Message:       encodedContent, // Restores the base64 string format
		Time:          t,
		ReplyTo:       p.ReplyTo,
		Email:         p.Email,
		InitialVector: p.Iv,
		HotSauce:      p.HotSauce,
	}
}
