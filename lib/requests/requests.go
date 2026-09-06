package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"time"
)

type Client struct {
	client     http.Client
	maxRetries int
}

func Init(timeout int) *Client {
	return &Client{
		client:     http.Client{Timeout: time.Duration(timeout) * time.Second},
		maxRetries: 3,
	}
}

// Do 通用请求，body 由 Body 接口拼装，响应 JSON 解码到 result。
func (c *Client) Do(method, rawURL string, body Body, result any, headers map[string]string) error {
	raw, err := c.DoBytes(method, rawURL, body, headers)
	if err != nil {
		return err
	}
	if result == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, result)
}

// DoBytes 返回原始字节，由调用方自行解析。
func (c *Client) DoBytes(method, rawURL string, body Body, headers map[string]string) ([]byte, error) {
	req, err := buildReq(method, rawURL, body, headers)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

// DoBytesTimeout 带单次超时的请求, 用于分片直传等慢通道。
func (c *Client) DoBytesTimeout(method, rawURL string, body Body, headers map[string]string, timeout time.Duration) ([]byte, error) {
	req, err := buildReq(method, rawURL, body, headers)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	return c.do(req.WithContext(ctx))
}

func buildReq(method, rawURL string, body Body, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s request: %w", method, err)
	}
	if body != nil {
		r, ct := body.Reader()
		req.Body = io.NopCloser(r)
		req.ContentLength = body.Size()
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

func (c *Client) Get(url string, result any, headers map[string]string) error {
	return c.Do(http.MethodGet, url, nil, result, headers)
}

func (c *Client) Post(url string, body any, result any, headers map[string]string) error {
	return c.Do(http.MethodPost, url, JSON(body), result, headers)
}

func (c *Client) PostForm(url string, form url.Values, result any, headers map[string]string) error {
	return c.Do(http.MethodPost, url, Form(form), result, headers)
}

func (c *Client) PostMultipart(url string, mp *Multipart, result any, headers map[string]string) error {
	return c.Do(http.MethodPost, url, mp, result, headers)
}

func (c *Client) Put(url string, body any, result any, headers map[string]string) error {
	return c.Do(http.MethodPut, url, ByteBody(body), result, headers)
}

func (c *Client) Patch(url string, body any, result any, headers map[string]string) error {
	return c.Do(http.MethodPatch, url, JSON(body), result, headers)
}

func (c *Client) Delete(url string, body any, result any, headers map[string]string) error {
	return c.Do(http.MethodDelete, url, JSON(body), result, headers)
}

// --- Body 抽象 ---

// Body 可重读请求体，内部预缓冲，支持重试。
type Body interface {
	Reader() (io.Reader, string) // 返回 body 与 Content-Type
	Size() int64
}

type bytesBody struct{ data []byte }

func Bytes(b []byte) Body { return bytesBody{data: b} }

func (b bytesBody) Reader() (io.Reader, string) { return bytes.NewReader(b.data), "application/json" }
func (b bytesBody) Size() int64                 { return int64(len(b.data)) }

// JSON 构造 JSON body：[]byte/string 原样，其他 json.Marshal。
func JSON(v any) Body {
	switch b := v.(type) {
	case nil:
		return nil
	case []byte:
		return Bytes(b)
	case string:
		return Bytes([]byte(b))
	case io.Reader:
		data, _ := io.ReadAll(b)
		return bytesBody{data: data}
	default:
		data, err := json.Marshal(v)
		if err != nil {
			panic(fmt.Sprintf("requests.JSON: %v", err))
		}
		return bytesBody{data: data}
	}
}

type formBody struct{ data []byte }

func Form(v url.Values) Body {
	return formBody{data: []byte(v.Encode())}
}

func (b formBody) Reader() (io.Reader, string) {
	return bytes.NewReader(b.data), "application/x-www-form-urlencoded"
}
func (b formBody) Size() int64 { return int64(len(b.data)) }

// ByteBody 将 any 转字节 body：[]byte 原样、string 转字节、其他 JSON 编码。
func ByteBody(v any) Body {
	switch b := v.(type) {
	case nil:
		return nil
	case []byte:
		return bytesBody{data: b}
	case string:
		return bytesBody{data: []byte(b)}
	default:
		return JSON(v)
	}
}

// Multipart 预构建 multipart/form-data 请求体。
type Multipart struct {
	buf  bytes.Buffer
	ct   string
	size int64
}

func NewMultipart() *Multipart { return &Multipart{} }

// AddField 添加表单字段。
func (mp *Multipart) AddField(key, value string) *Multipart {
	if err := mp.writer().WriteField(key, value); err != nil {
		panic(err)
	}
	return mp
}

// AddFile 添加文件字段。
func (mp *Multipart) AddFile(field, name, mimeType string, data []byte) *Multipart {
	mw := mp.writer()
	var part io.Writer
	if mimeType == "" {
		var err error
		part, err = mw.CreateFormFile(field, name)
		if err != nil {
			panic(err)
		}
	} else {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition",
			fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, path.Base(name)))
		h.Set("Content-Type", mimeType)
		var err error
		part, err = mw.CreatePart(h)
		if err != nil {
			panic(err)
		}
	}
	if _, err := part.Write(data); err != nil {
		panic(err)
	}
	return mp
}

// Close 完成 multipart 构造，此后可作 Body 使用。
func (mp *Multipart) Close() {
	mw := mp.writer()
	if err := mw.Close(); err != nil {
		panic(err)
	}
	mp.ct = mw.FormDataContentType()
	mp.size = int64(mp.buf.Len())
}

func (mp *Multipart) Reader() (io.Reader, string) {
	return bytes.NewReader(mp.buf.Bytes()), mp.ct
}

func (mp *Multipart) Size() int64 { return mp.size }

func (mp *Multipart) writer() *multipart.Writer { return multipart.NewWriter(&mp.buf) }

// StatusError 非 2xx 响应错误, 携带状态码供上层策略判断。
type StatusError struct {
	Code int
	Body []byte
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("unexpected status code: %d, body: %s", e.Code, e.Body)
}

func isRetryableStatus(code int) bool {
	return code == http.StatusRequestTimeout ||
		code == http.StatusTooManyRequests ||
		code >= 500
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}

	maxAttempts := c.maxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	delays := []time.Duration{200 * time.Millisecond, 300 * time.Millisecond, 500 * time.Millisecond}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := delays[len(delays)-1]
			if attempt-1 < len(delays) {
				delay = delays[attempt-1]
			}
			time.Sleep(delay)
		}

		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			req.ContentLength = int64(len(bodyBytes))
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(bodyBytes)), nil
			}
		} else {
			req.Body = nil
			req.ContentLength = 0
			req.GetBody = nil
		}

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = &StatusError{Code: resp.StatusCode, Body: respBody}
			if isRetryableStatus(resp.StatusCode) {
				continue
			}
			return nil, lastErr
		}

		return respBody, nil
	}

	return nil, lastErr
}
