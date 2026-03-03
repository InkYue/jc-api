package model

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

var requestLogOutputWorkerOnce sync.Once
var requestLogOutputQueue chan *RequestLogOutput

// StartRequestLogOutputWorker starts the async writer for request log outputs.
func StartRequestLogOutputWorker() {
	if !common.RequestLogEnabled || !common.RequestLogCaptureOutputEnabled || !common.RequestLogAsyncEnabled || LOG_DB == nil {
		return
	}
	if !isRequestLogOutputTableReady() {
		return
	}

	requestLogOutputWorkerOnce.Do(func() {
		queueSize := common.RequestLogAsyncQueueSize
		if queueSize <= 0 {
			queueSize = 4096
		}
		requestLogOutputQueue = make(chan *RequestLogOutput, queueSize)
		go runRequestLogOutputWorker()
		common.SysLog(fmt.Sprintf(
			"request log output async writer started (queue=%d, batch=%d, flush_ms=%d, redis_fallback=%t)",
			queueSize,
			common.RequestLogWriteBatchSize,
			common.RequestLogFlushIntervalMs,
			common.RequestLogRedisFallbackEnabled,
		))
	})
}

func enqueueRequestLogOutput(item *RequestLogOutput) {
	if item == nil {
		return
	}
	if requestLogOutputQueue == nil {
		if err := writeRequestLogOutputsBatch([]*RequestLogOutput{item}); err != nil {
			common.SysLog("failed to record request log output: " + err.Error())
		}
		return
	}

	select {
	case requestLogOutputQueue <- item:
		return
	default:
		if pushRequestLogOutputToRedis(item) {
			return
		}
		if common.DebugEnabled {
			common.SysLog("request log output queue is full and redis fallback failed, dropping output log")
		}
	}
}

func runRequestLogOutputWorker() {
	batchSize := common.RequestLogWriteBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	flushInterval := time.Duration(common.RequestLogFlushIntervalMs) * time.Millisecond
	if flushInterval <= 0 {
		flushInterval = 500 * time.Millisecond
	}

	pending := make([]*RequestLogOutput, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(pending) == 0 {
			return
		}
		if err := writeRequestLogOutputsBatch(pending); err != nil {
			common.SysLog("failed to batch write request log outputs: " + err.Error())
			for _, item := range pending {
				_ = pushRequestLogOutputToRedis(item)
			}
		}
		pending = pending[:0]
	}

	for {
		select {
		case item := <-requestLogOutputQueue:
			if item == nil {
				continue
			}
			pending = append(pending, item)
			if len(pending) >= batchSize {
				flush()
			}
		case <-ticker.C:
			if len(pending) < batchSize {
				drainRequestLogOutputsFromRedis(&pending, batchSize)
			}
			flush()
		}
	}
}

func pushRequestLogOutputToRedis(item *RequestLogOutput) bool {
	if !common.RequestLogRedisFallbackEnabled || !common.RedisEnabled || common.RDB == nil || item == nil {
		return false
	}
	payload, err := common.Marshal(item)
	if err != nil {
		common.SysLog("failed to marshal request log output for redis fallback: " + err.Error())
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = common.RDB.LPush(ctx, common.RequestLogOutputRedisQueueKey, payload).Err(); err != nil {
		common.SysLog("failed to push request log output to redis queue: " + err.Error())
		return false
	}
	return true
}

func drainRequestLogOutputsFromRedis(pending *[]*RequestLogOutput, maxBatchSize int) {
	if !common.RequestLogRedisFallbackEnabled || !common.RedisEnabled || common.RDB == nil {
		return
	}
	if len(*pending) >= maxBatchSize {
		return
	}

	drainLimit := common.RequestLogOutputRedisDrainBatch
	if drainLimit <= 0 {
		drainLimit = 200
	}

	for i := 0; i < drainLimit && len(*pending) < maxBatchSize; i++ {
		item, ok := popRequestLogOutputFromRedis()
		if !ok {
			break
		}
		*pending = append(*pending, item)
	}
}

func popRequestLogOutputFromRedis() (*RequestLogOutput, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	payload, err := common.RDB.RPop(ctx, common.RequestLogOutputRedisQueueKey).Result()
	if err != nil {
		if err != redis.Nil {
			common.SysLog("failed to pop request log output from redis queue: " + err.Error())
		}
		return nil, false
	}
	var item RequestLogOutput
	if err = common.Unmarshal([]byte(payload), &item); err != nil {
		common.SysLog("failed to unmarshal request log output from redis queue: " + err.Error())
		return nil, false
	}
	return &item, true
}
