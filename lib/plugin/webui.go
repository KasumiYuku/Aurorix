package plugin

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// WebUI 插件自带管理台的挂载声明: SPA 为前端产物根(dist), API 注册 /api/... 数据路由。
type WebUI struct {
	SPA fs.FS
	API func(mux *http.ServeMux)
}

// WebUIs 返回各插件声明的前端控制台, 键为插件 Id(即挂载前缀)。
func WebUIs() map[string]WebUI {
	lock.RLock()
	defer lock.RUnlock()
	out := make(map[string]WebUI, len(globalPlugins))
	for id, p := range globalPlugins {
		if p.WebUI != nil {
			out[id] = *p.WebUI
		}
	}
	return out
}

var assetExts = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".map": true, ".json": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".webp": true, ".avif": true, ".ico": true, ".woff": true, ".woff2": true,
	".ttf": true, ".otf": true, ".txt": true, ".webmanifest": true, ".xml": true,
}

// ServeSPA 托管单页应用: 命中文件按扩展名长缓存, 未命中的前端路由回退 index.html
func ServeSPA(fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		if name != "index.html" {
			if f, err := fsys.Open(name); err == nil {
				info, statErr := f.Stat()
				f.Close()
				if statErr == nil && !info.IsDir() {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					http.ServeFileFS(w, r, fsys, name)
					return
				}
			}
			if assetExts[strings.ToLower(path.Ext(name))] {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}
		index, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(index)
	}
}
