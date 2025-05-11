package model

import "time"

type User struct {
	ID        string    `json:"id"`         // 用户 ID
	Username  string    `json:"username"`   // 用户名
	Password  string    `json:"password"`   // 加密密码
	IsAdmin   bool      `json:"is_admin"`   // 是否管理员
	CreatedAt time.Time `json:"created_at"` // 创建时间
}
