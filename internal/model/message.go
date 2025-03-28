package model

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzar/chatlog/internal/model/wxproto"
	"github.com/sjzar/chatlog/pkg/util/lz4"

	"google.golang.org/protobuf/proto"
)

const (
	// Source
	WeChatV3       = "wechatv3"
	WeChatV4       = "wechatv4"
	WeChatDarwinV3 = "wechatdarwinv3"
)

type Message struct {
	Sequence        int64     `json:"sequence"`        // 消息序号，10位时间戳 + 3位序号
	CreateTime      time.Time `json:"createTime"`      // 消息创建时间，10位时间戳
	TalkerID        int       `json:"talkerID"`        // 聊天对象，Name2ID 表序号，索引值
	Talker          string    `json:"talker"`          // 聊天对象，微信 ID or 群 ID
	IsSender        int       `json:"isSender"`        // 是否为发送消息，0 接收消息，1 发送消息
	Type            int64     `json:"type"`            // 消息类型
	SubType         int       `json:"subType"`         // 消息子类型
	Content         string    `json:"content"`         // 消息内容，文字聊天内容 或 XML
	CompressContent []byte    `json:"compressContent"` // 非文字聊天内容，如图片、语音、视频等
	IsChatRoom      bool      `json:"isChatRoom"`      // 是否为群聊消息
	ChatRoomSender  string    `json:"chatRoomSender"`  // 群聊消息发送人

	// Fill Info
	// 从联系人等信息中填充
	DisplayName  string        `json:"-"` // 显示名称
	ChatRoomName string        `json:"-"` // 群聊名称
	MediaMessage *MediaMessage `json:"-"` // 多媒体消息

	Version string `json:"-"` // 消息版本，内部判断
}

// CREATE TABLE MSG (
// localId INTEGER PRIMARY KEY AUTOINCREMENT,
// TalkerId INT DEFAULT 0,
// MsgSvrID INT,
// Type INT,
// SubType INT,
// IsSender INT,
// CreateTime INT,
// Sequence INT DEFAULT 0,
// StatusEx INT DEFAULT 0,
// FlagEx INT,
// Status INT,
// MsgServerSeq INT,
// MsgSequence INT,
// StrTalker TEXT,
// StrContent TEXT,
// DisplayContent TEXT,
// Reserved0 INT DEFAULT 0,
// Reserved1 INT DEFAULT 0,
// Reserved2 INT DEFAULT 0,
// Reserved3 INT DEFAULT 0,
// Reserved4 TEXT,
// Reserved5 TEXT,
// Reserved6 TEXT,
// CompressContent BLOB,
// BytesExtra BLOB,
// BytesTrans BLOB
// )
type MessageV3 struct {
	Sequence        int64  `json:"Sequence"`        // 消息序号，10位时间戳 + 3位序号
	CreateTime      int64  `json:"CreateTime"`      // 消息创建时间，10位时间戳
	TalkerID        int    `json:"TalkerId"`        // 聊天对象，Name2ID 表序号，索引值
	StrTalker       string `json:"StrTalker"`       // 聊天对象，微信 ID or 群 ID
	IsSender        int    `json:"IsSender"`        // 是否为发送消息，0 接收消息，1 发送消息
	Type            int64  `json:"Type"`            // 消息类型
	SubType         int    `json:"SubType"`         // 消息子类型
	StrContent      string `json:"StrContent"`      // 消息内容，文字聊天内容 或 XML
	CompressContent []byte `json:"CompressContent"` // 非文字聊天内容，如图片、语音、视频等
	BytesExtra      []byte `json:"BytesExtra"`      // protobuf 额外数据，记录群聊发送人等信息

	// 非关键信息，后续有需要再加入
	// LocalID        int64  `json:"localId"`
	// MsgSvrID       int64  `json:"MsgSvrID"`
	// StatusEx       int    `json:"StatusEx"`
	// FlagEx         int    `json:"FlagEx"`
	// Status         int    `json:"Status"`
	// MsgServerSeq   int64  `json:"MsgServerSeq"`
	// MsgSequence    int64  `json:"MsgSequence"`
	// DisplayContent string `json:"DisplayContent"`
	// Reserved0      int    `json:"Reserved0"`
	// Reserved1      int    `json:"Reserved1"`
	// Reserved2      int    `json:"Reserved2"`
	// Reserved3      int    `json:"Reserved3"`
	// Reserved4      string `json:"Reserved4"`
	// Reserved5      string `json:"Reserved5"`
	// Reserved6      string `json:"Reserved6"`
	// BytesTrans     []byte `json:"BytesTrans"`
}

func (m *MessageV3) Wrap() *Message {

	_m := &Message{
		Sequence:        m.Sequence,
		CreateTime:      time.Unix(m.CreateTime, 0),
		TalkerID:        m.TalkerID,
		Talker:          m.StrTalker,
		IsSender:        m.IsSender,
		Type:            m.Type,
		SubType:         m.SubType,
		Content:         m.StrContent,
		CompressContent: m.CompressContent,
		Version:         WeChatV3,
	}

	_m.IsChatRoom = strings.HasSuffix(_m.Talker, "@chatroom")

	if _m.Type == 49 {
		b, err := lz4.Decompress(m.CompressContent)
		if err == nil {
			_m.Content = string(b)
		}
	}

	if _m.Type != 1 {
		mediaMessage, err := NewMediaMessage(_m.Type, _m.Content)
		if err == nil {
			_m.MediaMessage = mediaMessage
		}
	}

	if len(m.BytesExtra) != 0 {
		if bytesExtra := ParseBytesExtra(m.BytesExtra); bytesExtra != nil {
			if _m.IsChatRoom {
				_m.ChatRoomSender = bytesExtra[1]
			}
			// FIXME xml 中的 md5 数据无法匹配到 hardlink 记录，所以直接用 proto 数据
			if _m.Type == 43 {
				path := bytesExtra[4]
				parts := strings.Split(filepath.ToSlash(path), "/")
				if len(parts) > 1 {
					path = strings.Join(parts[1:], "/")
				}
				_m.MediaMessage.MediaPath = path
			}
		}
	}

	return _m
}

// ParseBytesExtra 解析额外数据
// 按需解析
func ParseBytesExtra(b []byte) map[int]string {
	var pbMsg wxproto.BytesExtra
	if err := proto.Unmarshal(b, &pbMsg); err != nil {
		return nil
	}
	if pbMsg.Items == nil {
		return nil
	}

	ret := make(map[int]string, len(pbMsg.Items))
	for _, item := range pbMsg.Items {
		ret[int(item.Type)] = item.Value
	}

	return ret
}

func (m *Message) PlainText(showChatRoom bool, host string) string {
	buf := strings.Builder{}

	talker := m.Talker
	if m.IsSender == 1 {
		talker = "我"
	} else if m.IsChatRoom {
		talker = m.ChatRoomSender
	}
	if m.DisplayName != "" {
		buf.WriteString(m.DisplayName)
		buf.WriteString("(")
		buf.WriteString(talker)
		buf.WriteString(")")
	} else {
		buf.WriteString(talker)
	}
	buf.WriteString(" ")

	if m.IsChatRoom && showChatRoom {
		buf.WriteString("[")
		if m.ChatRoomName != "" {
			buf.WriteString(m.ChatRoomName)
			buf.WriteString("(")
			buf.WriteString(m.Talker)
			buf.WriteString(")")
		} else {
			buf.WriteString(m.Talker)
		}
		buf.WriteString("] ")
	}

	buf.WriteString(m.CreateTime.Format("2006-01-02 15:04:05"))
	buf.WriteString("\n")

	if m.MediaMessage != nil {
		m.MediaMessage.SetHost(host)
		buf.WriteString(m.MediaMessage.String())
	} else {
		buf.WriteString(m.Content)
	}

	buf.WriteString("\n")

	return buf.String()
}
