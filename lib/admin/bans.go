package admin

import (
	"net/http"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/ban"
)

func registerBanRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /admin/api/bans", handleBanList)
	api.HandleFunc("POST /admin/api/bans", handleBanPut)
	api.HandleFunc("DELETE /admin/api/bans/{openid}", handleBanDelete)
}

func handleBanList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, H{
		"entries": ban.List(),
		"default": ban.DefaultMessage,
		"limit":   ban.MessageLimit,
	})
}

func handleBanPut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OpenID  string `json:"openid"`
		Message string `json:"message"`
		Note    string `json:"note"`
	}
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
		return
	}
	if err := ban.Put(ban.Entry{OpenID: in.OpenID, Message: in.Message, Note: in.Note}); err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, H{"ok": true, "entries": ban.List()})
}

func handleBanDelete(w http.ResponseWriter, r *http.Request) {
	openid := strings.TrimSpace(r.PathValue("openid"))
	if err := ban.Delete(openid); err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, H{"ok": true, "entries": ban.List()})
}
