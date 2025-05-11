package model

import "time"

type Image struct {
	ID          string    `json:"id"`          // MD5 值
	UserID      string    `json:"user_id"`     // 上传用户 ID
	Filename    string    `json:"filename"`    // 文件名
	Description string    `json:"description"` // 描述
	Tags        []string  `json:"tags"`        // 标签
	IsPrivate   bool      `json:"is_private"`  // 是否私有
	Views       int64     `json:"views"`       // 访问次数
	CreatedAt   time.Time `json:"created_at"`  // 上传时间
}
