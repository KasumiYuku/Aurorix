package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"path"
	"time"
)

type Client struct {
	client         *http.Client
	jar            http.CookieJar
	maxRetries     int
	defaultTimeout time.Duration
}

// Init 建客户端: 30x 自动跟随, 并带一个内存 cookie 罐 —— 上游下发一次凭证后续请求自动带上
func Init(timeout int) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		client:         &http.Client{Jar: jar},
		jar:            jar,
		maxRetries:     3,
		defaultTimeout: time.Duration(timeout) * time.Second,
	}
}

// NoRedirect 返回不跟随重定向的副本: 跳转响应原样返回(3xx 不再算错误), cookie 与重试配置共享。
func (c *Client) NoRedirect() *Client {
	clone := *c
	inner := *c.client
	inner.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	clone.client = &inner
	return &clone
}

// Do 通用请求，body 由 Body 接口拼装，响应 JSON 解码到 result。
func (c *Client) Do(method, rawURL string, body Body, result any, headers map[string]string) error {
	resp, err := c.DoResponse(method, rawURL, body, headers)
	if err != nil {
		return err
	}
	return resp.JSON(result)
}

// DoResponse 通用请求, 返回完整回执: 状态码、响应头、最终地址、正文与下发的 cookie。
func (c *Client) DoResponse(method, rawURL string, body Body, headers map[string]string) (*Response, error) {
	req, err := buildReq(method, rawURL, body, headers)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

// DoBytes 返回原始字节，由调用方自行解析。
func (c *Client) DoBytes(method, rawURL string, body Body, headers map[string]string) ([]byte, error) {
	resp, err := c.DoResponse(method, rawURL, body, headers)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// DoResponseTimeout 带单次超时的请求, 用于分片直传等慢通道。
func (c *Client) DoResponseTimeout(method, rawURL string, body Body, headers map[string]string, timeout time.Duration) (*Response, error) {
	req, err := buildReq(method, rawURL, body, headers)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	return c.do(req.WithContext(ctx))
}

// DoBytesTimeout 同 DoBytes, 单次超时覆盖客户端默认值。
func (c *Client) DoBytesTimeout(method, rawURL string, body Body, headers map[string]string, timeout time.Duration) ([]byte, error) {
	resp, err := c.DoResponseTimeout(method, rawURL, body, headers, timeout)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Cookie 读 cookie 罐里该地址当前携带的指定 cookie; 没有返回空串。
func (c *Client) Cookie(rawURL, name string) string {
	if c.jar == nil {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	for _, ck := range c.jar.Cookies(u) {
		if ck.Name == name {
			return ck.Value
		}
	}
	return ""
}

// SetCookie 往 cookie 罐里塞一个值, 供调用方预置登录态(配置里填的 cookie 走这里)。
func (c *Client) SetCookie(rawURL, name, value string) error {
	if c.jar == nil {
		return errors.New("requests: cookie 罐未初始化")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	c.jar.SetCookies(u, []*http.Cookie{{Name: name, Value: value, Path: "/"}})
	return nil
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

// Body 可重读请求体，内部预缓冲，支持重试。
type Body interface {
	Reader() (io.Reader, string)
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
	mw   *multipart.Writer
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

func (mp *Multipart) writer() *multipart.Writer {
	if mp.mw == nil {
		mp.mw = multipart.NewWriter(&mp.buf)
	}
	return mp.mw
}

// StatusError 非 2xx 响应错误, 携带状态码与地址供上层策略判断。
type StatusError struct {
	Code int
	URL  string
	Body []byte
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("unexpected status code %d from %s, body: %s", e.Code, e.URL, e.Body)
}

func isRetryableStatus(code int) bool {
	return code == http.StatusRequestTimeout ||
		code == http.StatusTooManyRequests ||
		code >= 500
}

func (c *Client) do(req *http.Request) (*Response, error) {
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}

	if _, ok := req.Context().Deadline(); !ok && c.defaultTimeout > 0 {
		ctx, cancel := context.WithTimeout(req.Context(), c.defaultTimeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	maxAttempts := max(c.maxRetries+1, 1)
	delays := []time.Duration{200 * time.Millisecond, 300 * time.Millisecond, 500 * time.Millisecond}
	callerRedirects := c.client.CheckRedirect != nil

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

		raw, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		respBody, readErr := io.ReadAll(raw.Body)
		out := &Response{
			StatusCode: raw.StatusCode,
			Header:     raw.Header,
			URL:        raw.Request.URL.String(),
			Body:       respBody,
			SetCookies: raw.Cookies(),
		}
		raw.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", readErr)
			continue
		}

		if raw.StatusCode < 200 || raw.StatusCode >= 300 {
			if callerRedirects && raw.StatusCode < 400 {
				return out, nil
			}
			lastErr = &StatusError{Code: raw.StatusCode, URL: out.URL, Body: respBody}
			if isRetryableStatus(raw.StatusCode) {
				continue
			}
			return nil, lastErr
		}

		return out, nil
	}

	return nil, lastErr
}
