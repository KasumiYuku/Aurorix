package providers

import (
	"context"
	"fmt"

	"github.com/KasumiYuku/Aurorix/lib/assets"
)

func init() {
	assets.Register("chatglm", func(cl *assets.Client, _ map[string]any) (assets.ImageProvider, error) {
		return &chatglmProvider{cl: cl}, nil
	}, nil)
}

type chatglmProvider struct{ cl *assets.Client }

func (p *chatglmProvider) Name() string { return "chatglm" }

func (p *chatglmProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	mp := assets.NewMultipart()
	mp.AddFile("file", in.Filename, in.MimeType, in.Buffer)
	mp.Close()
	var resp struct {
		Result struct {
			FileURL  string `json:"file_url"`
			FileName string `json:"file_name"`
		} `json:"result"`
		Message string `json:"message"`
	}
	err := p.cl.PostMultipart("https://chatglm.cn/chatglm/backend-api/assistant/file_upload",
		mp, &resp, map[string]string{
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		})
	if err != nil {
		return "", err
	}
	if resp.Result.FileURL == "" {
		return "", fmt.Errorf("chatglm: %s", resp.Message)
	}
	return resp.Result.FileURL, nil
}
