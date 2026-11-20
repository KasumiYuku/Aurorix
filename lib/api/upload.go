package api

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/requests"
)

// MediaUpload 媒体上传参数。
type MediaUpload struct {
	FileType     int
	Data         []byte
	Filename     string
	MimeType     string
	URL          string
	ForceChunked bool
}

// MediaResult 媒体上传结果。
type MediaResult struct {
	FileInfo string
	FileUUID string
	TTL      int
	RawURL   string
}

type mediaUploadResponse struct {
	FileInfo string `json:"file_info"`
	FileUUID string `json:"file_uuid"`
	TTL      int    `json:"ttl"`
	RawURL   string `json:"raw_url"`
}

func (s Session) mediaBase() (string, error) {
	switch s.Target.Scene {
	case constant.SceneGroup:
		return "/v2/groups/" + s.Target.ID, nil
	case constant.SceneC2C:
		return "/v2/users/" + s.Target.ID, nil
	}
	return "", &Unsupported{Scene: s.Target.Scene, What: "富媒体上传"}
}

// UploadImage 上传本地图片到平台, 返回 file_info。
func (s Session) UploadImage(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read image for upload: %w", err)
	}
	return s.Upload(MediaUpload{FileType: constant.MediaImage, Data: data, Filename: filePath})
}

// Upload 上传媒体, 返回 file_info。超过阈值自动分片。
func (s Session) Upload(up MediaUpload) (string, error) {
	res, err := s.UploadEx(up)
	if err != nil {
		return "", err
	}
	return res.FileInfo, nil
}

// UploadEx 同 Upload, 额外返回官方直链等字段。
func (s Session) UploadEx(up MediaUpload) (MediaResult, error) {
	if err := s.ready(); err != nil {
		return MediaResult{}, err
	}
	base, err := s.mediaBase()
	if err != nil {
		return MediaResult{}, err
	}
	if len(up.Data) > 0 && (up.ForceChunked || len(up.Data) > s.api.Threshold()) {
		return s.chunkedUpload(base, up)
	}

	body := map[string]any{
		"file_type":    up.FileType,
		"srv_send_msg": false,
	}
	switch {
	case up.URL != "":
		body["url"] = up.URL
	case len(up.Data) > 0:
		body["file_data"] = base64.StdEncoding.EncodeToString(up.Data)
	default:
		return MediaResult{}, fmt.Errorf("upload media: both url and data empty")
	}
	if up.Filename != "" {
		body["file_name"] = up.Filename
	}

	var result mediaUploadResponse
	raw, err := s.api.doWithin("POST", s.api.ProxyAPI+base+"/files", requests.JSON(body), uploadBudget(len(up.Data)))
	if err != nil {
		return MediaResult{}, fmt.Errorf("upload media to QQ: %w", err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return MediaResult{}, err
	}
	if result.FileInfo == "" {
		return MediaResult{}, fmt.Errorf("QQ media upload returned empty file_info")
	}
	return MediaResult{
		FileInfo: result.FileInfo, FileUUID: result.FileUUID,
		TTL: result.TTL, RawURL: result.RawURL,
	}, nil
}

func (s Session) chunkedUpload(base string, up MediaUpload) (MediaResult, error) {
	prepEndpoint := s.api.ProxyAPI + base + "/upload_prepare"
	finishEndpoint := s.api.ProxyAPI + base + "/upload_part_finish"
	sendEndpoint := s.api.ProxyAPI + base + "/files"

	type uploadPrepareRequest struct {
		FileType int    `json:"file_type"`
		FileName string `json:"file_name"`
		FileSize int    `json:"file_size"`
		MD5      string `json:"md5"`
		SHA1     string `json:"sha1"`
		// MD5_10M 前 10MB 的 md5: 官方用它判重, 缺这个键 QQ 会把每次上传都当新文件重新转码。
		MD5_10M string `json:"md5_10m"`
	}
	var prep struct {
		UploadID  string `json:"upload_id"`
		BlockSize string `json:"block_size"`
		Parts     []struct {
			Index        int    `json:"index"`
			PresignedURL string `json:"presigned_url"`
		} `json:"parts"`
	}
	raw, err := s.api.doWithin("POST", prepEndpoint, requests.JSON(uploadPrepareRequest{
		FileType: up.FileType, FileName: mediaFileName(up.FileType, up.Filename), FileSize: len(up.Data),
		MD5: md5HexBytes(up.Data), SHA1: sha1HexBytes(up.Data),
		MD5_10M: md5HexBytes(headBytes(up.Data, md5HeadSize)),
	}), uploadBudget(len(up.Data)))
	if err != nil {
		return MediaResult{}, fmt.Errorf("upload prepare: %w", err)
	}
	if err := json.Unmarshal(raw, &prep); err != nil {
		return MediaResult{}, err
	}

	blockSize, _ := strconv.Atoi(prep.BlockSize)
	if blockSize <= 0 {
		blockSize = 1024 * 1024
	}
	partHeader := map[string]string{}
	if up.MimeType != "" {
		partHeader["Content-Type"] = up.MimeType
	}
	for _, part := range prep.Parts {
		start := (part.Index - 1) * blockSize
		end := min(start+blockSize, len(up.Data))
		chunk := up.Data[start:end]

		last := error(nil)
		for attempt := 1; attempt <= partAttempts; attempt++ {
			if last = s.uploadPart(part.PresignedURL, finishEndpoint, prep.UploadID, part.Index, chunk, partHeader); last == nil {
				break
			}
			if attempt < partAttempts {
				time.Sleep(partRetryDelay * time.Duration(attempt))
			}
		}
		if last != nil {
			return MediaResult{}, fmt.Errorf("upload part %d 试了 %d 次: %w", part.Index, partAttempts, last)
		}
	}

	type sendFileRequest struct {
		UploadID   string `json:"upload_id"`
		FileType   int    `json:"file_type"`
		FileName   string `json:"file_name,omitempty"`
		SrvSendMsg bool   `json:"srv_send_msg"`
	}
	var result mediaUploadResponse
	raw, err = s.api.doWithin("POST", sendEndpoint, requests.JSON(sendFileRequest{
		UploadID: prep.UploadID, FileType: up.FileType, FileName: mediaFileName(up.FileType, up.Filename),
	}), uploadBudget(len(up.Data)))
	if err != nil {
		return MediaResult{}, fmt.Errorf("send chunked upload: %w", err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return MediaResult{}, err
	}
	if result.FileInfo == "" {
		return MediaResult{}, fmt.Errorf("chunked upload returned empty file_info")
	}
	return MediaResult{
		FileInfo: result.FileInfo, FileUUID: result.FileUUID,
		TTL: result.TTL, RawURL: result.RawURL,
	}, nil
}

func (s Session) uploadPart(presigned, finishEndpoint, uploadID string, index int, chunk []byte, headers map[string]string) error {
	if _, err := s.api.Request.DoBytesTimeout("PUT", presigned, requests.Bytes(chunk), headers, partTimeout(len(chunk))); err != nil {
		return fmt.Errorf("upload part %d: %w", index, err)
	}
	type partFinishRequest struct {
		UploadID  string `json:"upload_id"`
		PartIndex int    `json:"part_index"`
		BlockSize int    `json:"block_size"`
		MD5       string `json:"md5"`
	}
	if _, err := s.api.doWithin("POST", finishEndpoint, requests.JSON(partFinishRequest{
		UploadID: uploadID, PartIndex: index, BlockSize: len(chunk), MD5: md5HexBytes(chunk),
	}), uploadBudget(len(chunk))); err != nil {
		return fmt.Errorf("upload part finish %d: %w", index, err)
	}
	return nil
}

const (
	partAttempts   = 3
	partRetryDelay = 800 * time.Millisecond
)

func partTimeout(size int) time.Duration {
	return sizeBudget(size, time.Minute, 10*time.Minute, 128<<10)
}

func uploadBudget(size int) time.Duration {
	return sizeBudget(size, time.Minute, 10*time.Minute, 256<<10)
}

func sizeBudget(size int, floor, ceiling time.Duration, rate int) time.Duration {
	if size < 0 {
		size = 0
	}
	return min(max(floor+time.Duration(size/rate)*time.Second, floor), ceiling)
}

func md5HexBytes(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}

func sha1HexBytes(b []byte) string {
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}

func mediaFileName(fileType int, name string) string {
	if name == "" || path.Ext(name) != "" {
		return name
	}
	switch fileType {
	case constant.MediaVideo:
		return name + ".mp4"
	case constant.MediaVoice:
		return name + ".mp3"
	}
	return name
}

const md5HeadSize = 10_002_432

func headBytes(data []byte, n int) []byte {
	if len(data) <= n {
		return data
	}
	return data[:n]
}
