package api

// StreamAPI 流式消息域: 一条消息会话内连续分片更新, 直到完成。

import (
	"encoding/json"
	"fmt"

	"Plrx/lib/requests"
)

// StreamInputState 流式消息输入状态。
type StreamInputState int

const (
	StreamNotStream  StreamInputState = 0  // 非流式
	StreamGenerating StreamInputState = 1  // 生成中
	StreamDone       StreamInputState = 10 // 完成
)

// StreamInputMode 流式消息输入模式。
type StreamInputMode string

const StreamReplace StreamInputMode = "replace"

// StreamContentType 流式消息内容类型。
type StreamContentType string

const StreamMarkdown StreamContentType = "markdown"

// StreamMessage 流式消息分片请求。
type StreamMessage struct {
	InputMode   StreamInputMode   `json:"input_mode,omitempty"`
	InputState  StreamInputState  `json:"input_state"`
	ContentType StreamContentType `json:"content_type"`
	ContentRaw  string            `json:"content_raw"`
	EventID     string            `json:"event_id"`
	MsgID       string            `json:"msg_id"`
	StreamMsgID string            `json:"stream_msg_id,omitempty"`
	MsgSeq      uint8             `json:"msg_seq"`
	Index       uint32            `json:"index"` // 同一条流式会话内从 0 递增的发送索引
}

// SendStreamResult 流式消息发送结果。
type SendStreamResult struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
}

type StreamAPI struct {
	api *BotAPI
}

// SendStreamMessage 发送一条流式消息分片到私聊。
func (s *StreamAPI) SendStreamMessage(userID string, msg StreamMessage) (*SendStreamResult, error) {
	endpoint := fmt.Sprintf("%v/v2/users/%v/stream_messages", s.api.ProxyAPI, userID)
	var result SendStreamResult
	raw, err := s.api.do("POST", endpoint, requests.JSON(msg))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.ID == "" {
		return nil, fmt.Errorf("stream message returned empty id")
	}
	return &result, nil
}

// StreamSession 管理一条流式消息会话的连续分片发送。
type StreamSession struct {
	client   *BotAPI
	userID   string
	eventID  string
	msgID    string
	streamID string
	index    uint32
}

// NewStreamSession 创建流式消息会话。eventID/msgID 为触发事件的被动消息标识。
func (s *StreamAPI) NewStreamSession(userID, eventID, msgID string) *StreamSession {
	return &StreamSession{client: s.api, userID: userID, eventID: eventID, msgID: msgID}
}

// StreamID 已建立会话的流式消息 ID（首次发送后可用）。
func (s *StreamSession) StreamID() string { return s.streamID }

// SendContent 发送一段 markdown 内容。
func (s *StreamSession) SendContent(content string, inputState StreamInputState) error {
	s.index++
	msg := StreamMessage{
		InputMode:   StreamReplace,
		InputState:  inputState,
		ContentType: StreamMarkdown,
		ContentRaw:  content,
		EventID:     s.eventID,
		MsgID:       s.msgID,
		StreamMsgID: s.streamID,
		MsgSeq:      uint8(s.index),
		Index:       s.index - 1,
	}
	res, err := s.client.Stream.SendStreamMessage(s.userID, msg)
	if err != nil {
		return err
	}
	// 首次成功后记录流式消息 ID，供后续分片延续同一会话
	if s.streamID == "" {
		s.streamID = res.ID
	}
	return nil
}

// Update 更新当前流式消息。
func (s *StreamSession) Update(content string) error {
	return s.SendContent(content, StreamGenerating)
}

// Finish 结束流式消息。
func (s *StreamSession) Finish(content string) error {
	return s.SendContent(content, StreamDone)
}
