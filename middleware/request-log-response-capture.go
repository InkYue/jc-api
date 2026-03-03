package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

type responseCaptureWriter struct {
	gin.ResponseWriter
	maxCaptureBytes int
	totalBytes      int
	captured        []byte
	captureDisabled bool
}

func newResponseCaptureWriter(writer gin.ResponseWriter, maxCaptureBytes int) *responseCaptureWriter {
	if maxCaptureBytes < 0 {
		maxCaptureBytes = 0
	}
	return &responseCaptureWriter{
		ResponseWriter:  writer,
		maxCaptureBytes: maxCaptureBytes,
		captured:        make([]byte, 0, minInt(maxCaptureBytes, 4096)),
	}
}

func (w *responseCaptureWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.record(data[:safeSliceLen(n, len(data))])
	return n, err
}

func (w *responseCaptureWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	w.record([]byte(s[:safeSliceLen(n, len(s))]))
	return n, err
}

func (w *responseCaptureWriter) record(data []byte) {
	if len(data) == 0 {
		return
	}
	w.totalBytes += len(data)
	if w.maxCaptureBytes == 0 || w.captureDisabled {
		return
	}

	contentType := normalizeContentType(w.Header().Get("Content-Type"))
	if contentType != "" && !isTextLikeContentType(contentType) {
		w.captureDisabled = true
		w.captured = w.captured[:0]
		return
	}

	remaining := w.maxCaptureBytes - len(w.captured)
	if remaining <= 0 {
		return
	}
	if len(data) > remaining {
		data = data[:remaining]
	}
	w.captured = append(w.captured, data...)
}

func (w *responseCaptureWriter) capturedText() string {
	if len(w.captured) == 0 {
		return ""
	}
	return string(w.captured)
}

func (w *responseCaptureWriter) isCapturedTruncated() bool {
	return w.totalBytes > len(w.captured)
}

func safeSliceLen(n int, total int) int {
	if n <= 0 {
		return 0
	}
	if n > total {
		return total
	}
	return n
}

func normalizeContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	return contentType
}

func isTextLikeContentType(contentType string) bool {
	contentType = normalizeContentType(contentType)
	if contentType == "" {
		return false
	}
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	if strings.HasPrefix(contentType, "application/json") ||
		strings.HasSuffix(contentType, "+json") ||
		strings.HasPrefix(contentType, "application/xml") ||
		strings.HasSuffix(contentType, "+xml") ||
		strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(contentType, "application/javascript") ||
		strings.HasPrefix(contentType, "application/problem+json") ||
		strings.HasPrefix(contentType, "application/graphql-response+json") ||
		strings.HasPrefix(contentType, "application/ndjson") {
		return true
	}
	return false
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
