package api

import (
	"augment2api/config"
	"augment2api/pkg/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// CFWorker Cloudflare Workers配置结构
type CFWorker struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	WorkerURL   string `json:"worker_url"`
	Description string `json:"description"`
	Status      string `json:"status"` // active, inactive
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// CFWorkerRequest 创建/更新Workers的请求结构
type CFWorkerRequest struct {
	Name        string `json:"name" binding:"required"`
	WorkerURL   string `json:"worker_url" binding:"required"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// CFWorkerTestRequest 测试Workers的请求结构
type CFWorkerTestRequest struct {
	WorkerURL string `json:"worker_url" binding:"required"`
	TestPath  string `json:"test_path"`
}

// GetCFWorkers 获取所有Cloudflare Workers配置
func GetCFWorkers(c *gin.Context) {
	keys, err := config.RedisKeys("cf_worker:*")
	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Error("获取CF Workers配置失败")
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "获取配置失败",
		})
		return
	}

	workers := make([]CFWorker, 0, len(keys))
	for _, key := range keys {
		worker, err := getCFWorkerFromRedis(key)
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"key":   key,
				"error": err.Error(),
			}).Error("解析CF Worker配置失败")
			continue
		}
		workers = append(workers, worker)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"workers": workers,
		"total":   len(workers),
	})
}

// CreateCFWorker 创建新的Cloudflare Workers配置
func CreateCFWorker(c *gin.Context) {
	var req CFWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "请求数据格式错误: " + err.Error(),
		})
		return
	}

	// 生成新的ID
	id, err := generateCFWorkerID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "生成ID失败",
		})
		return
	}

	// 设置默认状态
	if req.Status == "" {
		req.Status = "active"
	}

	// 创建Workers配置
	worker := CFWorker{
		ID:          id,
		Name:        req.Name,
		WorkerURL:   req.WorkerURL,
		Description: req.Description,
		Status:      req.Status,
		CreatedAt:   time.Now().Format("2006-01-02 15:04:05"),
		UpdatedAt:   time.Now().Format("2006-01-02 15:04:05"),
	}

	err = saveCFWorkerToRedis(worker)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "保存配置失败",
		})
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"id":   id,
		"name": req.Name,
		"url":  req.WorkerURL,
	}).Info("创建CF Worker配置成功")

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"worker": worker,
	})
}

// UpdateCFWorker 更新Cloudflare Workers配置
func UpdateCFWorker(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的ID",
		})
		return
	}

	var req CFWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "请求数据格式错误: " + err.Error(),
		})
		return
	}

	// 获取现有配置
	key := fmt.Sprintf("cf_worker:%d", id)
	worker, err := getCFWorkerFromRedis(key)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "配置不存在",
		})
		return
	}

	// 更新配置
	worker.Name = req.Name
	worker.WorkerURL = req.WorkerURL
	worker.Description = req.Description
	if req.Status != "" {
		worker.Status = req.Status
	}
	worker.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")

	err = saveCFWorkerToRedis(worker)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "更新配置失败",
		})
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"id":   id,
		"name": req.Name,
		"url":  req.WorkerURL,
	}).Info("更新CF Worker配置成功")

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"worker": worker,
	})
}

// DeleteCFWorker 删除Cloudflare Workers配置
func DeleteCFWorker(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的ID",
		})
		return
	}

	key := fmt.Sprintf("cf_worker:%d", id)
	
	// 检查配置是否存在
	exists, err := config.RedisExists(key)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "配置不存在",
		})
		return
	}

	// 删除配置
	err = config.RedisDel(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "删除配置失败",
		})
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"id": id,
	}).Info("删除CF Worker配置成功")

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "配置删除成功",
	})
}

// TestCFWorker 测试Cloudflare Workers连接
func TestCFWorker(c *gin.Context) {
	var req CFWorkerTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "请求数据格式错误: " + err.Error(),
		})
		return
	}

	// 设置默认测试路径
	testPath := req.TestPath
	if testPath == "" {
		testPath = "/v1/models"
	}

	// 构建测试URL
	testURL := req.WorkerURL + testPath

	// 发送测试请求
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(testURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  "error",
			"error":   "连接失败: " + err.Error(),
			"success": false,
		})
		return
	}
	defer resp.Body.Close()

	// 检查响应状态
	success := resp.StatusCode >= 200 && resp.StatusCode < 400

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"success":     success,
		"status_code": resp.StatusCode,
		"test_url":    testURL,
		"message":     fmt.Sprintf("测试完成，状态码: %d", resp.StatusCode),
	})
}

// 辅助函数

// generateCFWorkerID 生成新的CF Worker ID
func generateCFWorkerID() (int, error) {
	// 获取当前最大ID
	keys, err := config.RedisKeys("cf_worker:*")
	if err != nil {
		return 0, err
	}

	maxID := 0
	for _, key := range keys {
		// 从key中提取ID (格式: "cf_worker:123")
		var id int
		if _, err := fmt.Sscanf(key, "cf_worker:%d", &id); err == nil {
			if id > maxID {
				maxID = id
			}
		}
	}

	return maxID + 1, nil
}

// getCFWorkerFromRedis 从Redis获取CF Worker配置
func getCFWorkerFromRedis(key string) (CFWorker, error) {
	var worker CFWorker

	data, err := config.RedisGet(key)
	if err != nil {
		return worker, err
	}

	err = json.Unmarshal([]byte(data), &worker)
	return worker, err
}

// saveCFWorkerToRedis 保存CF Worker配置到Redis
func saveCFWorkerToRedis(worker CFWorker) error {
	key := fmt.Sprintf("cf_worker:%d", worker.ID)
	data, err := json.Marshal(worker)
	if err != nil {
		return err
	}

	return config.RedisSet(key, string(data), 0) // 永不过期
}
