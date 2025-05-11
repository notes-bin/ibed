package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/notes-bin/ibed/internal/model"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	*redis.Client
}

func (c *Client) GetCachedImage(context context.Context, param any) (any, any) {
	panic("unimplemented")
}

func NewClient(addr, password string, db, poolSize int) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		PoolSize: poolSize,
	})
	_, err := client.Ping(context.Background()).Result()
	if err != nil {
		return nil, err
	}
	slog.Info("Connected to Redis")
	return &Client{client}, nil
}

func (c *Client) SaveUser(ctx context.Context, user *model.User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return c.Set(ctx, fmt.Sprintf("user:%s", user.ID), data, 0).Err()
}

func (c *Client) GetUser(ctx context.Context, username string) (*model.User, error) {
	data, err := c.Get(ctx, fmt.Sprintf("user:%s", username)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var user model.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) SaveImage(ctx context.Context, img *model.Image) error {
	data, err := json.Marshal(img)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("image:%s", img.ID)

	// 使用事务保存图片元数据和标签
	_, err = c.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		// 保存图片元数据
		pipe.Set(ctx, key, data, 0)

		// 保存标签
		for _, tag := range img.Tags {
			pipe.SAdd(ctx, fmt.Sprintf("image:%s:tags", img.ID), tag)
		}

		// 添加到用户图片列表
		pipe.SAdd(ctx, fmt.Sprintf("user:%s:images", img.UserID), img.ID)

		// 设置过期时间
		pipe.Expire(ctx, key, 30*24*time.Hour)
		pipe.Expire(ctx, fmt.Sprintf("image:%s:tags", img.ID), 30*24*time.Hour)
		pipe.Expire(ctx, fmt.Sprintf("user:%s:images", img.UserID), 30*24*time.Hour)

		return nil
	})
	return err
}

func (c *Client) GetImage(ctx context.Context, imageID string) (*model.Image, error) {
	// 获取图片元数据
	data, err := c.Get(ctx, fmt.Sprintf("image:%s", imageID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var img model.Image
	if err := json.Unmarshal(data, &img); err != nil {
		return nil, err
	}

	// 获取标签
	tags, err := c.SMembers(ctx, fmt.Sprintf("image:%s:tags", imageID)).Result()
	if err != nil {
		return nil, err
	}
	img.Tags = tags

	return &img, nil
}

func (c *Client) IncrementView(ctx context.Context, imageID string) error {
	return c.ZIncrBy(ctx, "image:views", 1, imageID).Err()
}

func (c *Client) GetTop10Images(ctx context.Context) ([]string, error) {
	return c.ZRevRange(ctx, "image:views", 0, 9).Result()
}

func (c *Client) SearchImages(ctx context.Context, query string, offset, limit int) ([]*model.Image, error) {
	// 简单实现：扫描所有图片，匹配描述或标签
	// 生产环境建议使用 Redis Search 模块
	images := []*model.Image{}
	keys, err := c.Keys(ctx, "image:*").Result()
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		img, err := c.GetImage(ctx, key[6:]) // 去掉 "image:" 前缀
		if err != nil {
			continue
		}
		if containsQuery(img.Description, query) || containsTags(img.Tags, query) {
			images = append(images, img)
		}
	}
	start := offset
	end := offset + limit
	if start >= len(images) {
		return []*model.Image{}, nil
	}
	if end > len(images) {
		end = len(images)
	}
	return images[start:end], nil
}

func containsQuery(str, query string) bool {
	return strings.Contains(strings.ToLower(str), strings.ToLower(query))
}

func containsTags(tags []string, query string) bool {
	query = strings.ToLower(query)
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

// 添加缓存方法
func (c *Client) CacheUser(ctx context.Context, user *model.User, ttl time.Duration) error {
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return c.Set(ctx, fmt.Sprintf("cache:user:%s", user.ID), data, ttl).Err()
}

func (c *Client) GetUserFromCache(ctx context.Context, username string) (*model.User, error) {
	data, err := c.Get(ctx, fmt.Sprintf("cache:user:%s", username)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var user model.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, err
	}
	return &user, nil
}
