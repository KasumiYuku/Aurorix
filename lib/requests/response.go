package requests

import (
	"encoding/json"
	"net/http"
)

// Response 一次请求的完整回执: 状态码、响应头、最终地址、正文与本次下发的 cookie。
type Response struct {
	StatusCode int
	Header     http.Header
	URL        string
	Body       []byte
	SetCookies []*http.Cookie
}

// JSON 解码正文到 v; 空正文或 v 为 nil 时不报错。
func (r *Response) JSON(v any) error {
	if v == nil || len(r.Body) == 0 {
		return nil
	}
	return json.Unmarshal(r.Body, v)
}

// Text 正文当文本用。
func (r *Response) Text() string { return string(r.Body) }

// Cookie 取本次响应下发的指定 cookie; 没有返回空串。
func (r *Response) Cookie(name string) string {
	for _, c := range r.SetCookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}
