package message

import "fmt"

type JSONMarshalError struct {
	Err error
}

func (e *JSONMarshalError) Error() string {
	return fmt.Sprintf("Failed when marshal: %v", e.Err)
}

func (e *JSONMarshalError) Unwrap() error {
	return e.Err
}

type ReplyMessageReachLimit struct{}

func (e *ReplyMessageReachLimit) Error() string {
	return "Reply message count reached limit: 5"
}
