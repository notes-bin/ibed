package auth

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/notes-bin/ibed/internal/model"
	"github.com/notes-bin/ibed/internal/redis"

	"github.com/golang-jwt/jwt/v5"
)

type Auth struct {
	secret string
	redis  *redis.Client
}

func NewAuth(secret string, redis *redis.Client) *Auth {
	return &Auth{secret: secret, redis: redis}
}

func (a *Auth) Register(ctx context.Context, username, password string) (*model.User, error) {
	// 检查用户名是否已存在
	existingUser, err := a.redis.GetUser(ctx, username)
	if err != nil {
		return nil, err
	}
	if existingUser != nil {
		return nil, fmt.Errorf("username already exists")
	}

	hashed := a.HashPassword(password)
	user := &model.User{
		ID:        username, // 使用用户名作为ID
		Username:  username,
		Password:  hashed,
		IsAdmin:   username == "admin", // 首次注册 admin 为超级管理员
		CreatedAt: time.Now(),
	}
	if err := a.redis.SaveUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (a *Auth) HashPassword(password string) string {
	hash := md5.Sum([]byte(password))
	return hex.EncodeToString(hash[:])
}

func (a *Auth) Login(ctx context.Context, username, password string, expiresIn time.Duration) (string, error) {
	user, err := a.redis.GetUser(ctx, username)
	if err != nil {
		return "", err
	}
	if user == nil || user.Password != a.HashPassword(password) {
		return "", fmt.Errorf("invalid credentials")
	}
	return a.GenerateToken(user.ID, user.Username, user.IsAdmin, expiresIn)
}

func (a *Auth) GenerateToken(userID, username string, isAdmin bool, expiresIn time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"is_admin": isAdmin,
		"exp":      time.Now().Add(expiresIn).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(a.secret))
}

func (a *Auth) ChangePassword(ctx context.Context, username, newPassword string) error {
	user, err := a.redis.GetUser(ctx, username)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// 更新密码
	user.Password = a.HashPassword(newPassword)
	if err := a.redis.SaveUser(ctx, user); err != nil {
		return err
	}

	return nil
}
