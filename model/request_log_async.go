package model

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

var requestLogWorkerOnce sync.Once
var requestLogQueue chan *RequestLog

func StartRequestLogWorker() {
	if !common.RequestLogEnabled || !common.RequestLogAsyncEnabled || LOG_DB == nil {
		return
	}
	if !isRequestLogTableReady() {
		return
	}

	requestLogWorkerOnce.Do(func() {
		queueSize := common.RequestLogAsyncQueueSize
		if queueSize <= 0 {
			queueSize = 4096
		}
		requestLogQueue = make(chan *RequestLog, queueSize)
		go runRequestLogWorker()
		common.SysLog(fmt.Sprintf(
			"request log async writer started (queue=%d, batch=%d, flush_ms=%d, redis_fallback=%t)",
			queueSize,
			common.RequestLogWriteBatchSize,
			common.RequestLogFlushIntervalMs,
			common.RequestLogRedisFallbackEnabled,
		))
	})
}

func enqueueRequestLog(log *RequestLog) {
	if log == nil {
		return
	}
	if requestLogQueue == nil {
		if err := writeRequestLogsBatch([]*RequestLog{log}); err != nil {
			common.SysLog("failed to record request log: " + err.Error())
		}
		return
	}

	select {
	case requestLogQueue <- log:
		return
	default:
		if pushRequestLogToRedis(log) {
			return
		}
		if common.DebugEnabled {
			common.SysLog("request log queue is full and redis fallback failed, dropping log")
		}
	}
}

func runRequestLogWorker() {
	batchSize := common.RequestLogWriteBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	flushInterval := time.Duration(common.RequestLogFlushIntervalMs) * time.Millisecond
	if flushInterval <= 0 {
		flushInterval = 500 * time.Millisecond
	}

	pending := make([]*RequestLog, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(pending) == 0 {
			return
		}
		if err := writeRequestLogsBatch(pending); err != nil {
			common.SysLog("failed to batch write request logs: " + err.Error())
			for _, item := range pending {
				_ = pushRequestLogToRedis(item)
			}
		}
		pending = pending[:0]
	}

	for {
		select {
		case item := <-requestLogQueue:
			if item == nil {
				continue
			}
			pending = append(pending, item)
			if len(pending) >= batchSize {
				flush()
			}
		case <-ticker.C:
			if len(pending) < batchSize {
				drainRequestLogsFromRedis(&pending, batchSize)
			}
			flush()
		}
	}
}

func pushRequestLogToRedis(log *RequestLog) bool {
	if !common.RequestLogRedisFallbackEnabled || !common.RedisEnabled || common.RDB == nil || log == nil {
		return false
	}
	payload, err := common.Marshal(log)
	if err != nil {
		common.SysLog("failed to marshal request log for redis fallback: " + err.Error())
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = common.RDB.LPush(ctx, common.RequestLogRedisQueueKey, payload).Err(); err != nil {
		common.SysLog("failed to push request log to redis queue: " + err.Error())
		return false
	}
	return true
}

func drainRequestLogsFromRedis(pending *[]*RequestLog, maxBatchSize int) {
	if !common.RequestLogRedisFallbackEnabled || !common.RedisEnabled || common.RDB == nil {
		return
	}
	if len(*pending) >= maxBatchSize {
		return
	}

	drainLimit := common.RequestLogRedisDrainBatch
	if drainLimit <= 0 {
		drainLimit = 200
	}

	for i := 0; i < drainLimit && len(*pending) < maxBatchSize; i++ {
		item, ok := popRequestLogFromRedis()
		if !ok {
			break
		}
		*pending = append(*pending, item)
	}
}

func popRequestLogFromRedis() (*RequestLog, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	payload, err := common.RDB.RPop(ctx, common.RequestLogRedisQueueKey).Result()
	if err != nil {
		if err != redis.Nil {
			common.SysLog("failed to pop request log from redis queue: " + err.Error())
		}
		return nil, false
	}
	var item RequestLog
	if err = common.Unmarshal([]byte(payload), &item); err != nil {
		common.SysLog("failed to unmarshal request log from redis queue: " + err.Error())
		return nil, false
	}
	return &item, true
}
