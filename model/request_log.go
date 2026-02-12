package model

import (
	"errors"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type RequestLog struct {
	Id              int    `json:"id" gorm:"index:idx_request_logs_created_at_id,priority:1;index:idx_request_logs_user_id_id,priority:2"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint;index:idx_request_logs_created_at_id,priority:2;index"`
	RequestId       string `json:"request_id" gorm:"type:varchar(64);index:idx_request_logs_request_id;default:''"`
	UserId          int    `json:"user_id" gorm:"index;index:idx_request_logs_user_id_id,priority:1"`
	Username        string `json:"username" gorm:"type:varchar(64);index;default:''"`
	TokenId         int    `json:"token_id" gorm:"index;default:0"`
	Group           string `json:"group" gorm:"type:varchar(64);index;default:''"`
	ModelName       string `json:"model_name" gorm:"type:varchar(128);index;default:''"`
	ChannelId       int    `json:"channel_id" gorm:"index;default:0"`
	ChannelName     string `json:"channel_name" gorm:"type:varchar(128);default:''"`
	Method          string `json:"method" gorm:"type:varchar(12);index;default:''"`
	Path            string `json:"path" gorm:"type:varchar(512);index;default:''"`
	Query           string `json:"query" gorm:"type:text"`
	StatusCode      int    `json:"status_code" gorm:"index;default:0"`
	DurationMs      int64  `json:"duration_ms" gorm:"default:0"`
	ClientIP        string `json:"client_ip" gorm:"type:varchar(64);index;default:''"`
	UserAgent       string `json:"user_agent" gorm:"type:text"`
	BodyBytes       int    `json:"body_bytes" gorm:"default:0"`
	IsBodyTruncated bool   `json:"is_body_truncated" gorm:"default:false"`
	RequestBody     string `json:"request_body" gorm:"type:text"`
}

type RecordRequestLogParams struct {
	RequestId       string
	UserId          int
	Username        string
	TokenId         int
	Group           string
	ModelName       string
	ChannelId       int
	ChannelName     string
	Method          string
	Path            string
	Query           string
	StatusCode      int
	DurationMs      int64
	ClientIP        string
	UserAgent       string
	BodyBytes       int
	IsBodyTruncated bool
	RequestBody     string
}

var requestLogTableCheck sync.Once
var requestLogTableReady bool

func (RequestLog) TableName() string {
	return "request_logs"
}

// InitRequestLogTable initializes request_logs table only when it does not exist.
// It intentionally avoids AutoMigrate to prevent schema migration on large databases.
func InitRequestLogTable() error {
	if !common.RequestLogEnabled || LOG_DB == nil {
		return nil
	}
	if LOG_DB.Migrator().HasTable(&RequestLog{}) {
		requestLogTableReady = true
		return nil
	}
	if err := LOG_DB.Migrator().CreateTable(&RequestLog{}); err != nil {
		return err
	}
	requestLogTableReady = true
	common.SysLog("request_logs table initialized")
	return nil
}

func RecordRequestLog(params RecordRequestLogParams) {
	if !common.RequestLogEnabled || LOG_DB == nil {
		return
	}
	if !isRequestLogTableReady() {
		return
	}
	log := newRequestLogEntity(params)
	if common.RequestLogAsyncEnabled {
		enqueueRequestLog(log)
		return
	}
	if err := writeRequestLogsBatch([]*RequestLog{log}); err != nil {
		common.SysLog("failed to record request log: " + err.Error())
	}
}

func newRequestLogEntity(params RecordRequestLogParams) *RequestLog {
	return &RequestLog{
		CreatedAt:       common.GetTimestamp(),
		RequestId:       truncateByBytes(params.RequestId, 64),
		UserId:          params.UserId,
		Username:        truncateByBytes(params.Username, 64),
		TokenId:         params.TokenId,
		Group:           truncateByBytes(params.Group, 64),
		ModelName:       truncateByBytes(params.ModelName, 128),
		ChannelId:       params.ChannelId,
		ChannelName:     truncateByBytes(params.ChannelName, 128),
		Method:          truncateByBytes(params.Method, 12),
		Path:            truncateByBytes(params.Path, 512),
		Query:           truncateByBytes(params.Query, 4096),
		StatusCode:      params.StatusCode,
		DurationMs:      params.DurationMs,
		ClientIP:        truncateByBytes(params.ClientIP, 64),
		UserAgent:       truncateByBytes(params.UserAgent, 1024),
		BodyBytes:       params.BodyBytes,
		IsBodyTruncated: params.IsBodyTruncated,
		RequestBody:     params.RequestBody,
	}
}

func writeRequestLogsBatch(logs []*RequestLog) error {
	if len(logs) == 0 {
		return nil
	}
	batchSize := common.RequestLogWriteBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	return LOG_DB.CreateInBatches(logs, batchSize).Error
}

func isRequestLogTableReady() bool {
	if requestLogTableReady {
		return true
	}
	requestLogTableCheck.Do(func() {
		requestLogTableReady = LOG_DB.Migrator().HasTable(&RequestLog{})
		if !requestLogTableReady {
			common.SysLog("request_logs table not found, skip request log persistence")
		}
	})
	return requestLogTableReady
}

func truncateByBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes]
}

func GetRequestLogs(startTimestamp int64, endTimestamp int64, requestID string, method string, path string, statusCode int, username string, startIdx int, num int) (logs []*RequestLog, total int64, err error) {
	if LOG_DB == nil {
		return nil, 0, errors.New("log database is not initialized")
	}
	if !LOG_DB.Migrator().HasTable(&RequestLog{}) {
		return []*RequestLog{}, 0, nil
	}

	tx := LOG_DB.Model(&RequestLog{})

	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if requestID != "" {
		tx = tx.Where("request_id = ?", requestID)
	}
	if method != "" {
		tx = tx.Where("method = ?", strings.ToUpper(method))
	}
	if path != "" {
		pathPattern, pathErr := sanitizeLikePattern(path)
		if pathErr != nil {
			return nil, 0, pathErr
		}
		tx = tx.Where("path LIKE ? ESCAPE '!'", pathPattern)
	}
	if statusCode != 0 {
		tx = tx.Where("status_code = ?", statusCode)
	}
	if username != "" {
		tx = tx.Where("username = ?", username)
	}

	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
