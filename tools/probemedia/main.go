// 探针：逐个图床传同一个媒体文件，看谁真的能吃视频。
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"encoding/json"

	"github.com/KasumiYuku/Aurorix/lib/assets"
	_ "github.com/KasumiYuku/Aurorix/lib/assets/providers"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("用法: probemedia <assets.json> <媒体文件> [mime]")
		os.Exit(2)
	}
	mime := "video/mp4"
	if len(os.Args) > 3 {
		mime = os.Args[3]
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("读配置失败:", err)
		os.Exit(1)
	}
	var cfg assets.HostConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fmt.Println("解析配置失败:", err)
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Println("读媒体失败:", err)
		os.Exit(1)
	}
	name := os.Args[2]
	if i := lastSlash(name); i >= 0 {
		name = name[i+1:]
	}
	fmt.Printf("媒体: %s  %d 字节  %s\n\n", name, len(data), mime)

	cl := assets.NewClient(90)
	for _, item := range cfg.Providers {
		if item.Enabled != nil && !*item.Enabled {
			continue
		}
		p, err := assets.Instantiate(item.Name, cl, item.Config)
		if err != nil {
			fmt.Printf("%-12s ⚪ 构造失败: %v\n", item.Name, err)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		start := time.Now()
		url, err := p.Upload(ctx, assets.ProviderInput{
			Buffer: data, Filename: name, MimeType: mime, Origin: "probe",
		})
		cancel()
		cost := time.Since(start).Milliseconds()
		if err != nil {
			fmt.Printf("%-12s ❌ %dms  %v\n", item.Name, cost, err)
			continue
		}
		fmt.Printf("%-12s ✅ %dms  %s\n", item.Name, cost, url)
	}
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' || s[i] == '\\' {
			return i
		}
	}
	return -1
}
