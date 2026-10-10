package api

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/requests"
	"os"
	"strconv"
	"time"
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

type FilesAPI struct {
	api *BotAPI
}

// UploadImage 上传本地图片到 QQ, 返回 file_info。
func (f *FilesAPI) UploadImage(target constant.MessageOrigin, groupID, userID, filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read image for upload: %w", err)
	}
	return f.UploadMedia(target, groupID, userID, MediaUpload{FileType: 1, Data: data, Filename: filePath})
}

// UploadMedia 上传媒体到 QQ, 返回 file_info。超过阈值自动分片上传。
func (f *FilesAPI) UploadMedia(target constant.MessageOrigin, groupID, userID string, up MediaUpload) (string, error) {
	res, err := f.UploadMediaEx(target, groupID, userID, up)
	if err != nil {
		return "", err
	}
	return res.FileInfo, nil
}

// UploadMediaEx 同 UploadMedia, 额外返回官方直链等字段。
func (f *FilesAPI) UploadMediaEx(target constant.MessageOrigin, groupID, userID string, up MediaUpload) (MediaResult, error) {
	threshold := f.api.UploadThreshold
	if threshold <= 0 {
		threshold = 3 << 20
	}

	var endpoint string
	switch target {
	case constant.GroupMessage:
		endpoint = fmt.Sprintf("%v/v2/groups/%v/files", f.api.ProxyAPI, groupID)
	case constant.PrivateMessage:
		endpoint = fmt.Sprintf("%v/v2/users/%v/files", f.api.ProxyAPI, userID)
	default:
		return MediaResult{}, fmt.Errorf("unknown message target type: %v", target)
	}

	if len(up.Data) > 0 && (up.ForceChunked || len(up.Data) > threshold) {
		return f.chunkedUpload(target, groupID, userID, up)
	}

	body := map[string]any{
		"file_type":    up.FileType,
		"srv_send_msg": false,
	}
	if up.URL != "" {
		body["url"] = up.URL
	} else if len(up.Data) > 0 {
		body["file_data"] = base64.StdEncoding.EncodeToString(up.Data)
	} else {
		return MediaResult{}, fmt.Errorf("upload media: both url and data empty")
	}
	if up.Filename != "" {
		body["file_name"] = up.Filename
	}

	var result mediaUploadResponse
	raw, err := f.api.doWithin("POST", endpoint, requests.JSON(body), uploadBudget(len(up.Data)))
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

func (f *FilesAPI) chunkedUpload(target constant.MessageOrigin, groupID, userID string, up MediaUpload) (MediaResult, error) {
	var prepEndpoint, finishEndpoint string
	if target == constant.GroupMessage {
		prepEndpoint = fmt.Sprintf("%v/v2/groups/%v/upload_prepare", f.api.ProxyAPI, groupID)
		finishEndpoint = fmt.Sprintf("%v/v2/groups/%v/upload_part_finish", f.api.ProxyAPI, groupID)
	} else {
		prepEndpoint = fmt.Sprintf("%v/v2/users/%v/upload_prepare", f.api.ProxyAPI, userID)
		finishEndpoint = fmt.Sprintf("%v/v2/users/%v/upload_part_finish", f.api.ProxyAPI, userID)
	}

	type uploadPrepareRequest struct {
		FileType int    `json:"file_type"`
		FileName string `json:"file_name"`
		FileSize int    `json:"file_size"`
		MD5      string `json:"md5"`
		SHA1     string `json:"sha1"`
	}
	var prep struct {
		UploadID  string `json:"upload_id"`
		BlockSize string `json:"block_size"`
		Parts     []struct {
			Index        int    `json:"index"`
			PresignedURL string `json:"presigned_url"`
		} `json:"parts"`
	}
	raw, err := f.api.doWithin("POST", prepEndpoint, requests.JSON(uploadPrepareRequest{
		FileType: up.FileType, FileName: up.Filename, FileSize: len(up.Data),
		MD5: md5HexBytes(up.Data), SHA1: sha1HexBytes(up.Data),
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
		if _, err := f.api.Request.DoBytesTimeout("PUT", part.PresignedURL, requests.Bytes(chunk), partHeader, partTimeout(len(chunk))); err != nil {
			return MediaResult{}, fmt.Errorf("upload part %d: %w", part.Index, err)
		}
		type partFinishRequest struct {
			UploadID  string `json:"upload_id"`
			PartIndex int    `json:"part_index"`
			BlockSize int    `json:"block_size"`
			MD5       string `json:"md5"`
		}
		if _, err := f.api.doWithin("POST", finishEndpoint, requests.JSON(partFinishRequest{
			UploadID: prep.UploadID, PartIndex: part.Index,
			BlockSize: len(chunk), MD5: md5HexBytes(chunk),
		}), uploadBudget(len(chunk))); err != nil {
			return MediaResult{}, fmt.Errorf("upload part finish %d: %w", part.Index, err)
		}
	}

	type sendFileRequest struct {
		UploadID   string `json:"upload_id"`
		FileType   int    `json:"file_type"`
		FileName   string `json:"file_name,omitempty"`
		SrvSendMsg bool   `json:"srv_send_msg"`
	}
	var sendEndpoint string
	if target == constant.GroupMessage {
		sendEndpoint = fmt.Sprintf("%v/v2/groups/%v/files", f.api.ProxyAPI, groupID)
	} else {
		sendEndpoint = fmt.Sprintf("%v/v2/users/%v/files", f.api.ProxyAPI, userID)
	}
	var result mediaUploadResponse
	raw, err = f.api.doWithin("POST", sendEndpoint, requests.JSON(sendFileRequest{
		UploadID: prep.UploadID, FileType: up.FileType, FileName: up.Filename,
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
