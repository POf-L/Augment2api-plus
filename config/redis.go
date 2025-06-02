package config

import (
	"augment2api/pkg/logger"
	"context"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
)

var RDB redis.Cmdable

// InitRedisClient This function is called after init()
func InitRedisClient() (err error) {

	RedisConnString := AppConfig.RedisConnString
	if RedisConnString == "" {
		logger.Log.Debug("REDIS_CONN_STRING not set, Redis is not enabled")
		return nil
	}

	opt, err := redis.ParseURL(RedisConnString)
	if err != nil {
		logger.Log.Fatalln("failed to parse Redis connection string: " + err.Error())
	}
	RDB = redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = RDB.Ping(ctx).Result()
	if err != nil {
		logger.Log.Fatalln("Redis ping test failed: " + err.Error())
	}
	return err
}

func ParseRedisOption() *redis.Options {
	opt, err := redis.ParseURL(os.Getenv("REDIS_CONN_STRING"))
	if err != nil {
		logger.Log.Fatalln("failed to parse Redis connection string: " + err.Error())
	}
	return opt
}

func RedisSet(key string, value string, expiration time.Duration) error {
	ctx := context.Background()
	return RDB.Set(ctx, key, value, expiration).Err()
}

func RedisGet(key string) (string, error) {
	ctx := context.Background()
	return RDB.Get(ctx, key).Result()
}

func RedisDel(key string) error {
	ctx := context.Background()
	return RDB.Del(ctx, key).Err()
}

// RedisHSet 设置哈希表字段值
func RedisHSet(key, field, value string) error {
	ctx := context.Background()
	return RDB.HSet(ctx, key, field, value).Err()
}

// RedisHGet 获取哈希表字段值
func RedisHGet(key, field string) (string, error) {
	ctx := context.Background()
	return RDB.HGet(ctx, key, field).Result()
}

// RedisExpire 设置键的过期时间
func RedisExpire(key string, expiration time.Duration) error {
	ctx := context.Background()
	return RDB.Expire(ctx, key, expiration).Err()
}

// RedisKeys 获取匹配指定模式的所有键
func RedisKeys(pattern string) ([]string, error) {
	ctx := context.Background()
	return RDB.Keys(ctx, pattern).Result()
}

// RedisExists 检查键是否存在
func RedisExists(key string) (bool, error) {
	ctx := context.Background()
	result, err := RDB.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// RedisIncr 增加Redis中的计数器
func RedisIncr(key string) error {
	ctx := context.Background()
	_, err := RDB.Incr(ctx, key).Result()
	return err
}

// RedisHExists 检查哈希表字段是否存在
func RedisHExists(key, field string) (bool, error) {
	ctx := context.Background()
	return RDB.HExists(ctx, key, field).Result()
}

// RedisHGetAll 获取哈希表中的所有字段和值
func RedisHGetAll(key string) (map[string]string, error) {
	ctx := context.Background()
	return RDB.HGetAll(ctx, key).Result()
}
