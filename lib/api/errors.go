package api

import (
	"encoding/json"
	"errors"

	"github.com/KasumiYuku/Aurorix/lib/error"
	"github.com/KasumiYuku/Aurorix/lib/requests"
)

func asQQError(err error) *errorx.QQError {
	var qe *errorx.QQError
	if errors.As(err, &qe) {
		return qe
	}
	var se *requests.StatusError
	if errors.As(err, &se) {
		var body struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(se.Body, &body) == nil && body.Code != 0 {
			return errorx.New(body.Code, body.Message)
		}
	}
	return nil
}
