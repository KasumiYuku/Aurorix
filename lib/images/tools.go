package images

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

var dimClient = &http.Client{
	Timeout: 5 * time.Second,
}

const maxProbeBytes = 1 << 20

// GetImageDimensions 请求 URL 并返回图片宽高，失败返回零值与错误。
func GetImageDimensions(url string) (int, int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")

	resp, err := dimClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxProbeBytes))
	if err != nil {
		return 0, 0, err
	}
	sz := Probe(buf)
	if sz == nil {
		return 0, 0, fmt.Errorf("无法识别图片格式")
	}
	return sz.Width, sz.Height, nil
}

var fetchClient = &http.Client{
	Timeout: 15 * time.Second,
}

const maxFetchBytes = 4 << 20

// Fetch 把一条公网图片地址抓成字节。频道域必须走这条: image 字段虽然收 URL, 但那是平台自己去抓, 抓不到就连同正文整条丢掉。
func Fetch(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")

	resp, err := fetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
	if err != nil {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, fmt.Errorf("图片内容为空")
	}
	if len(buf) > maxFetchBytes {
		return nil, fmt.Errorf("图片超过 %d 字节上限", maxFetchBytes)
	}
	return buf, nil
}
