package main

import (
	"time"

	"github.com/rexlx/squall/internal"
	pb "github.com/rexlx/squall/proto"
)

// ToProto converts internal.Message to pb.ChatMessage
func ToProto(m internal.Message) *pb.ChatMessage {
	// Parse Time (String -> Int64)
	var ts int64
	parsedTime, err := time.Parse(time.RFC3339, m.Time)
	if err == nil {
		ts = parsedTime.Unix()
	} else {
		ts = time.Now().Unix()
	}

	// Content (Base64 String -> Bytes) in proto
	// If your proto uses 'string message_content' instead of 'bytes', remove the DecodeString part.
	// Assuming the latest proto uses 'string message_content':
	return &pb.ChatMessage{
		RoomId:         m.RoomID,
		UserId:         m.UserID,
		Email:          m.Email,
		MessageContent: m.Message, // Pass base64 string directly
		Timestamp:      ts,
		ReplyTo:        m.ReplyTo,
		Iv:             m.InitialVector,
		HotSauce:       m.HotSauce,
	}
}

// FromProto converts pb.ChatMessage to internal.Message
func FromProto(p *pb.ChatMessage) internal.Message {
	// Time (Int64 -> String)
	t := time.Unix(p.Timestamp, 0).Format(time.RFC3339)

	return internal.Message{
		RoomID:        p.RoomId,
		UserID:        p.UserId,
		Email:         p.Email,
		Message:       p.MessageContent, // Keep as base64 string
		Time:          t,
		ReplyTo:       p.ReplyTo,
		InitialVector: p.Iv,
		HotSauce:      p.HotSauce,
	}
}
