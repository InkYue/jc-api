package middleware

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// RequestLogDB persists request-level debug logs into LOG_DB.
func RequestLogDB() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.RequestLogEnabled {
			c.Next()
			return
		}
		if !shouldRecordRequestPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		startTime := time.Now()
		c.Next()

		if requestStartTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime); !requestStartTime.IsZero() {
			startTime = requestStartTime
		}

		userID := c.GetInt("id")
		username := c.GetString("username")
		if !common.ShouldRecordRequestLogForUser(userID, username) {
			return
		}

		body, bodyBytes, isTruncated := getRequestBodyPreview(c, common.RequestLogMaxBodyBytes)
		params := model.RecordRequestLogParams{
			RequestId:       c.GetString(common.RequestIdKey),
			UserId:          userID,
			Username:        username,
			TokenId:         c.GetInt("token_id"),
			Group:           c.GetString("group"),
			ModelName:       c.GetString("original_model"),
			ChannelId:       c.GetInt("channel_id"),
			ChannelName:     c.GetString("channel_name"),
			Method:          c.Request.Method,
			Path:            c.Request.URL.Path,
			Query:           c.Request.URL.RawQuery,
			StatusCode:      c.Writer.Status(),
			DurationMs:      time.Since(startTime).Milliseconds(),
			ClientIP:        c.ClientIP(),
			UserAgent:       c.Request.UserAgent(),
			BodyBytes:       bodyBytes,
			IsBodyTruncated: isTruncated,
			RequestBody:     body,
		}
		model.RecordRequestLog(params)
	}
}

func shouldRecordRequestPath(path string) bool {
	if strings.HasPrefix(path, "/v1") ||
		strings.HasPrefix(path, "/v1beta") ||
		strings.HasPrefix(path, "/pg/") ||
		strings.HasPrefix(path, "/mj/") ||
		path == "/mj" ||
		strings.HasPrefix(path, "/suno/") ||
		path == "/suno" ||
		strings.HasPrefix(path, "/kling/") ||
		strings.HasPrefix(path, "/jimeng") {
		return true
	}
	return false
}

func getRequestBodyPreview(c *gin.Context, maxBodyBytes int) (string, int, bool) {
	if maxBodyBytes <= 0 {
		return "", 0, false
	}
	if !shouldCaptureBody(c.Request.Method, c.GetHeader("Content-Type")) {
		return "", 0, false
	}
	bodyBytes, totalBytes, ok := getStoredRequestBody(c)
	if !ok || totalBytes <= 0 {
		return "", 0, false
	}
	isTruncated := false
	if totalBytes > maxBodyBytes {
		bodyBytes = bodyBytes[:maxBodyBytes]
		isTruncated = true
	}
	return string(bodyBytes), totalBytes, isTruncated
}

func shouldCaptureBody(method string, contentType string) bool {
	if method == "GET" || method == "HEAD" || method == "OPTIONS" {
		return false
	}
	contentType = strings.ToLower(contentType)
	if idx := strings.Index(contentType, ";"); idx > 0 {
		contentType = contentType[:idx]
	}
	if strings.HasPrefix(contentType, "application/json") ||
		strings.HasSuffix(contentType, "+json") ||
		strings.HasPrefix(contentType, "text/") ||
		strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
		strings.Contains(contentType, "xml") {
		return true
	}
	return false
}

func getStoredRequestBody(c *gin.Context) ([]byte, int, bool) {
	if stored, ok := c.Get(common.KeyBodyStorage); ok && stored != nil {
		if bs, ok := stored.(common.BodyStorage); ok {
			data, err := bs.Bytes()
			if err != nil {
				return nil, int(bs.Size()), false
			}
			return data, len(data), true
		}
	}
	if cached, ok := c.Get(common.KeyRequestBody); ok && cached != nil {
		if data, ok := cached.([]byte); ok {
			return data, len(data), true
		}
	}
	return nil, 0, false
}
