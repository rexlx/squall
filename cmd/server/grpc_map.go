package main

import (
	"fmt"
	"time"

	"github.com/rexlx/squall/internal"
	pb "github.com/rexlx/squall/proto"
)

func ToProto(m internal.Message) *pb.ChatMessage {
	var ts int64
	parsedTime, err := time.Parse(time.RFC3339, m.Time)
	if err == nil {
		ts = parsedTime.Unix()
	} else {
		ts = time.Now().Unix()
	}

	return &pb.ChatMessage{
		RoomId:    m.RoomID,
		UserId:    m.UserID,
		Email:     m.Email,
		Timestamp: ts,
		ReplyTo:   m.ReplyTo,
		Iv:        m.InitialVector,
		HotSauce:  m.HotSauce,
		Type:      pb.ChatMessage_TEXT,
		Payload: &pb.ChatMessage_MessageContent{
			MessageContent: m.Message,
		},
	}
}

func FromProto(p *pb.ChatMessage) internal.Message {
	t := time.Unix(p.Timestamp, 0).Format(time.RFC3339)

	content := ""
	if p.Type == pb.ChatMessage_TEXT {
		content = p.GetMessageContent()
	} else if p.Type == pb.ChatMessage_FILE_CONTROL {
		if meta := p.GetFileMeta(); meta != nil {
			content = fmt.Sprintf("FILE_CONTROL:%s", meta.Action)
		}
	}

	return internal.Message{
		RoomID:        p.RoomId,
		UserID:        p.UserId,
		Email:         p.Email,
		Message:       content,
		Time:          t,
		ReplyTo:       p.ReplyTo,
		InitialVector: p.Iv,
		HotSauce:      p.HotSauce,
	}
}
