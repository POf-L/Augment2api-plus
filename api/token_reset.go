package api

import (
	"augment2api/config"
	"augment2api/pkg/logger"

	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

// ResetTokenUsage 重置所有token的使用次数
func ResetTokenUsage() error {
	// 获取所有token的key
	keys, err := config.RedisKeys("token:*")
	if err != nil {
		return err
	}

	for _, key := range keys {
		// 从key中提取token
		token := key[6:] // 去掉前缀 "token:"

		// 重置总使用次数
		totalUsageKey := "token_usage:" + token
		err = config.RedisSet(totalUsageKey, "0", 0) // 0表示永不过期
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"token": token,
				"error": err,
			}).Error("重置Token总使用次数失败")
			continue
		}

		// 重置CHAT模式使用次数
		chatUsageKey := "token_usage_chat:" + token
		err = config.RedisSet(chatUsageKey, "0", 0) // 0表示永不过期
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"token": token,
				"error": err,
			}).Error("重置Token CHAT模式使用次数失败")
			continue
		}

		// 重置AGENT模式使用次数
		agentUsageKey := "token_usage_agent:" + token
		err = config.RedisSet(agentUsageKey, "0", 0) // 0表示永不过期
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"token": token,
				"error": err,
			}).Error("重置Token AGENT模式使用次数失败")
			continue
		}

		logger.Log.WithFields(logrus.Fields{
			"token": token,
		}).Info("重置token使用次数成功")
	}

	return nil
}

// StartTokenUsageResetScheduler 启动token使用次数重置调度器
func StartTokenUsageResetScheduler() {
	// 创建cron调度器
	c := cron.New(cron.WithSeconds()) // 启用秒级精度

	// 添加定时任务，每月1号零点一分执行
	// 格式：秒 分 时 日 月 周
	_, err := c.AddFunc("0 1 0 1 * *", func() {
		logger.Log.Info("开始执行Token使用次数重置任务")
		err := ResetTokenUsage()
		if err != nil {
			logger.Log.WithFields(logrus.Fields{
				"error": err,
			}).Error("执行Token使用次数重置任务失败")
		} else {
			logger.Log.Info("Token使用次数重置任务执行完成")
		}
	})

	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"error": err,
		}).Error("添加Token使用次数重置定时任务失败")
		return
	}

	// 启动cron调度器
	c.Start()
	logger.Log.Info("Token使用次数重置调度器启动成功!")

	// 保持程序运行
	select {}
}
