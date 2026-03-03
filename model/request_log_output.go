package model

import (
	"errors"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// RequestLogOutput stores captured response output text for a request.
type RequestLogOutput struct {
	Id              int    `json:"id" gorm:"index:idx_request_log_outputs_created_at_id,priority:1"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint;index:idx_request_log_outputs_created_at_id,priority:2;index"`
	RequestId       string `json:"request_id" gorm:"type:varchar(64);index:idx_request_log_outputs_request_id;default:''"`
	StatusCode      int    `json:"status_code" gorm:"index;default:0"`
	ContentType     string `json:"content_type" gorm:"type:varchar(255);default:''"`
	ResponseBytes   int    `json:"response_bytes" gorm:"default:0"`
	IsBinary        bool   `json:"is_binary" gorm:"default:false"`
	IsBodyTruncated bool   `json:"is_body_truncated" gorm:"default:false"`
	ResponseBody    string `json:"response_body" gorm:"type:text"`
}

// RecordRequestLogOutputParams describes response-output fields collected from middleware.
type RecordRequestLogOutputParams struct {
	RequestId       string
	StatusCode      int
	ContentType     string
	ResponseBytes   int
	IsBinary        bool
	IsBodyTruncated bool
	ResponseBody    string
}

var requestLogOutputTableCheck sync.Once
var requestLogOutputTableReady bool

// TableName returns the request log output table name.
func (RequestLogOutput) TableName() string {
	return "request_log_outputs"
}

// InitRequestLogOutputTable initializes request_log_outputs table only when it does not exist.
// It intentionally avoids AutoMigrate to prevent schema migration on large databases.
func InitRequestLogOutputTable() error {
	if !common.RequestLogEnabled || !common.RequestLogCaptureOutputEnabled || LOG_DB == nil {
		return nil
	}
	if LOG_DB.Migrator().HasTable(&RequestLogOutput{}) {
		requestLogOutputTableReady = true
		return nil
	}
	if err := LOG_DB.Migrator().CreateTable(&RequestLogOutput{}); err != nil {
		return err
	}
	requestLogOutputTableReady = true
	common.SysLog("request_log_outputs table initialized")
	return nil
}

// RecordRequestLogOutput persists one response output record.
func RecordRequestLogOutput(params RecordRequestLogOutputParams) {
	if !common.RequestLogEnabled || !common.RequestLogCaptureOutputEnabled || LOG_DB == nil {
		return
	}
	if params.RequestId == "" {
		return
	}
	if params.ResponseBytes <= 0 && params.ResponseBody == "" {
		return
	}
	if !isRequestLogOutputTableReady() {
		return
	}
	item := &RequestLogOutput{
		CreatedAt:       common.GetTimestamp(),
		RequestId:       truncateByBytes(params.RequestId, 64),
		StatusCode:      params.StatusCode,
		ContentType:     truncateByBytes(params.ContentType, 255),
		ResponseBytes:   params.ResponseBytes,
		IsBinary:        params.IsBinary,
		IsBodyTruncated: params.IsBodyTruncated,
		ResponseBody:    params.ResponseBody,
	}
	if common.RequestLogAsyncEnabled {
		enqueueRequestLogOutput(item)
		return
	}
	if err := writeRequestLogOutputsBatch([]*RequestLogOutput{item}); err != nil {
		common.SysLog("failed to record request log output: " + err.Error())
	}
}

func isRequestLogOutputTableReady() bool {
	if requestLogOutputTableReady {
		return true
	}
	requestLogOutputTableCheck.Do(func() {
		requestLogOutputTableReady = LOG_DB.Migrator().HasTable(&RequestLogOutput{})
		if !requestLogOutputTableReady {
			common.SysLog("request_log_outputs table not found, skip request log output persistence")
		}
	})
	return requestLogOutputTableReady
}

func writeRequestLogOutputsBatch(items []*RequestLogOutput) error {
	if len(items) == 0 {
		return nil
	}
	batchSize := common.RequestLogWriteBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	return LOG_DB.CreateInBatches(items, batchSize).Error
}

// GetRequestLogOutputByRequestID returns the latest output record for a request ID.
func GetRequestLogOutputByRequestID(requestID string) (*RequestLogOutput, error) {
	if LOG_DB == nil {
		return nil, errors.New("log database is not initialized")
	}
	if requestID == "" {
		return nil, errors.New("request_id is required")
	}
	if !LOG_DB.Migrator().HasTable(&RequestLogOutput{}) {
		return nil, nil
	}
	var output RequestLogOutput
	err := LOG_DB.Where("request_id = ?", requestID).Order("id desc").First(&output).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &output, nil
}
