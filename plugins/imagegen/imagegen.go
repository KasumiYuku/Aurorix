package imagegen

import (
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/requests"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var logger = logx.New("imagegen")

type Config struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	ModelID string `json:"model_id"`
}

type generationResponse struct {
	Data []struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
	} `json:"data"`
}

var (
	configMu       sync.RWMutex
	current        Config
	client         = requests.Init(120)
	downloadClient = &http.Client{Timeout: 60 * time.Second}
)

const maxImageDownloadSize int64 = 20 << 20

func init() {
	plugin.Register(&plugin.Plugin{
		Id:          "imagegen",
		Name:        "图像生成",
		Description: "OpenAI 兼容的图像生成接口",
		Config: []plugin.ConfigField{
			{Key: "base_url", Label: "Base URL", Description: "API 根地址，不包含 /images/generations。", Type: "text", Placeholder: "https://api.openai.com/v1"},
			{Key: "api_key", Label: "API Key", Description: "密钥不会在管理面板中回显。", Type: "password"},
			{Key: "model_id", Label: "Model ID", Description: "服务商提供的图像模型标识。", Type: "text", Placeholder: "gpt-image-1"},
		},
		ValidateConfig: validateConfig,
		ApplyConfig:    applyConfig,
		Commands: []*plugin.Command{{
			Prefix:   "draw",
			Role:     constant.RoleMember,
			Describe: "使用 OpenAI 兼容接口生成图片",
			Handle:   draw,
		}},
	})
}

func configFromValues(values map[string]any) Config {
	next := Config{}
	next.BaseURL, _ = values["base_url"].(string)
	next.APIKey, _ = values["api_key"].(string)
	next.ModelID, _ = values["model_id"].(string)
	next.BaseURL = strings.TrimRight(strings.TrimSpace(next.BaseURL), "/")
	next.APIKey = strings.TrimSpace(next.APIKey)
	next.ModelID = strings.TrimSpace(next.ModelID)
	return next
}

func validateConfig(values map[string]any) error {
	next := configFromValues(values)
	if next.BaseURL != "" && !strings.HasPrefix(next.BaseURL, "http://") && !strings.HasPrefix(next.BaseURL, "https://") {
		return fmt.Errorf("Base URL 必须以 http:// 或 https:// 开头")
	}
	return nil
}

func applyConfig(values map[string]any) error {
	next := configFromValues(values)
	SetConfig(next)
	return nil
}

func SetConfig(next Config) {
	next.BaseURL = strings.TrimRight(strings.TrimSpace(next.BaseURL), "/")
	next.ModelID = strings.TrimSpace(next.ModelID)
	configMu.Lock()
	current = next
	configMu.Unlock()
}

func GetConfig() Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return current
}

func draw(ctx *context.MessageContext) error {
	cfg := GetConfig()
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.ModelID == "" {
		return ctx.Text("生图配置不完整。请联系管理员检查接口配置。").Send()
	}

	prompt := strings.TrimSpace(ctx.Parsed.(string))
	if prompt == "" {
		content, err := templates.FillMarkdownTemplate("Card", templates.Args{
			"title": "❌ 指令参数错误",
			"fields": []any{
				map[string]any{"label": "用法", "content": "/draw <图片描述>"},
			},
		})
		if err != nil {
			return err
		}
		return ctx.Markdown(content).Send()
	}
	if err := ctx.Text("正在生成图片，请稍候...").Send(); err != nil {
		return fmt.Errorf("send image generation progress: %w", err)
	}

	endpoint := cfg.BaseURL + "/images/generations"
	body := map[string]any{
		"model":           cfg.ModelID,
		"prompt":          prompt,
		"n":               1,
		"response_format": "url",
	}
	if images := referenceImages(ctx.Attachments); len(images) > 0 {
		endpoint = cfg.BaseURL + "/images/edits"
		body["images"] = images
	}

	var result generationResponse
	err := client.Post(endpoint, body, &result, map[string]string{
		"Authorization": "Bearer " + cfg.APIKey,
	})
	if err != nil {
		_ = ctx.Text("图片生成失败，请稍后重试。").Send()
		return fmt.Errorf("image generation request failed: %w", err)
	}
	if len(result.Data) == 0 {
		_ = ctx.Text("图片生成接口未返回图片。").Send()
		return fmt.Errorf("image generation response contains no images")
	}

	image := result.Data[0]
	if image.URL == "" {
		_ = ctx.Text("图片接口未返回可发送的 URL，请将接口配置为 URL 输出。").Send()
		return fmt.Errorf("image generation response contains no url (base64 returned: %t)", image.B64JSON != "")
	}
	logger.Infof("generated image URL: %s", image.URL)
	filePath, err := downloadImage(image.URL)
	if err != nil {
		_ = ctx.Text("图片下载失败，请稍后重试。").Send()
		return err
	}
	defer os.Remove(filePath)

	fileInfo, err := ctx.Qapi.UploadImage(ctx.Target, ctx.GroupId, ctx.UserId, filePath)
	if err != nil {
		_ = ctx.Text("图片上传失败，请稍后重试。").Send()
		return err
	}
	return ctx.Media(fileInfo).Send()
}

func downloadImage(imageURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create image download request: %w", err)
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download generated image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download generated image: unexpected status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxImageDownloadSize {
		return "", fmt.Errorf("generated image exceeds %d bytes", maxImageDownloadSize)
	}

	file, err := os.CreateTemp("", "bot-generated-image-*")
	if err != nil {
		return "", fmt.Errorf("create temporary image: %w", err)
	}
	path := file.Name()
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()

	written, err := io.Copy(file, io.LimitReader(resp.Body, maxImageDownloadSize+1))
	if err != nil {
		return "", fmt.Errorf("save generated image: %w", err)
	}
	if written > maxImageDownloadSize {
		return "", fmt.Errorf("generated image exceeds %d bytes", maxImageDownloadSize)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close generated image: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("verify generated image: %w", err)
	}
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("generated file is not an image: %s", contentType)
	}
	keep = true
	return path, nil
}

func referenceImages(attachments []message.Attachment) []map[string]string {
	images := make([]map[string]string, 0, len(attachments))
	for _, attachment := range attachments {
		if len(images) == 16 {
			break
		}
		if !strings.HasPrefix(strings.ToLower(attachment.ContentType), "image/") {
			continue
		}
		url := strings.TrimSpace(attachment.URL)
		if strings.HasPrefix(url, "//") {
			url = "https:" + url
		}
		if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
			continue
		}
		images = append(images, map[string]string{"image_url": url})
	}
	return images
}
