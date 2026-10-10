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
