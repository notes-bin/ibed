package api

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/notes-bin/ibed/internal/auth"
	"github.com/notes-bin/ibed/internal/config"
	"github.com/notes-bin/ibed/internal/model"
	"github.com/notes-bin/ibed/internal/redis"
	"github.com/notes-bin/ibed/internal/storage"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	config  *config.Config
	auth    *auth.Auth
	redis   *redis.Client
	storage *storage.Storage
}

func NewHandler(config *config.Config, auth *auth.Auth, redis *redis.Client, storage *storage.Storage) *Handler {
	return &Handler{config: config, auth: auth, redis: redis, storage: storage}
}

func SetupRouter(config *config.Config, redis *redis.Client) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(RateLimitMiddleware(config.RateLimit.Requests, config.RateLimit.Duration))

	authService := auth.NewAuth(config.JWTSecret, redis)
	storageService, err := storage.NewStorage(config.UploadDir)
	if err != nil {
		slog.Error("Failed to initialize storage", "error", err)
		os.Exit(1)
	}
	h := NewHandler(config, authService, redis, storageService)

	// 公共路由
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)

	// 需要认证的路由
	r.Group(func(r chi.Router) {
		r.Use(h.AuthMiddleware)
		r.Post("/upload", h.UploadImage)
		r.Post("/batch-upload", h.BatchUploadImages)
		r.Delete("/image/{id}", h.DeleteImage)
		r.Post("/batch-delete", h.BatchDeleteImages)
		r.Post("/change-password", h.ChangePassword)
		r.Delete("/user", h.DeleteUser)
		r.Post("/refresh-token", h.RefreshToken)
		r.Get("/search", h.SearchImages)

		// 管理员路由
		r.Group(func(r chi.Router) {
			r.Use(h.AdminMiddleware)
			r.Get("/users", h.ListUsers)
			r.Post("/reset-password", h.ResetPassword)
			r.Post("/change-username", h.ChangeUsername)
		})
	})

	// 图片访问（支持公有和私有）
	r.Get("/image/{id}", h.GetImage)

	return r
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	user, err := h.auth.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to register")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "User registered", "user_id": user.ID})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	expiresIn := time.Duration(req.ExpiresIn) * time.Second
	if expiresIn == 0 {
		expiresIn = 24 * time.Hour
	}
	token, err := h.auth.Login(r.Context(), req.Username, req.Password, expiresIn)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	userID := r.Context().Value("user_id").(string)
	user, err := h.redis.GetUser(r.Context(), userID)
	if err != nil || user == nil {
		respondError(w, http.StatusInternalServerError, "User not found")
		return
	}
	user.Password = h.auth.HashPassword(req.NewPassword)
	if err := h.redis.SaveUser(r.Context(), user); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update password")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Password changed"})
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)
	isAdmin := r.Context().Value("is_admin").(bool)

	// 管理员删除其他用户
	if isAdmin {
		var req struct {
			TargetUserID string `json:"target_user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request")
			return
		}
		userID = req.TargetUserID
	}

	// 检查是否是管理员账户
	if user, err := h.redis.GetUser(r.Context(), userID); err == nil && user != nil && user.IsAdmin {
		respondError(w, http.StatusForbidden, "Cannot delete admin user")
		return
	}

	// 删除用户相关数据
	if err := h.redis.Del(r.Context(), fmt.Sprintf("user:%s", userID)).Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "User deleted"})
}

func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpiresIn int `json:"expires_in"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	userID := r.Context().Value("user_id").(string)
	username := r.Context().Value("username").(string)
	isAdmin := r.Context().Value("is_admin").(bool)
	expiresIn := time.Duration(req.ExpiresIn) * time.Second
	if expiresIn == 0 {
		expiresIn = 24 * time.Hour
	}
	token, err := h.auth.GenerateToken(userID, username, isAdmin, expiresIn)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to refresh token")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	keys, err := h.redis.Keys(r.Context(), "user:*").Result()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}
	users := []model.User{}
	for _, key := range keys {
		user, err := h.redis.GetUser(r.Context(), key[5:]) // 去掉 "user:" 前缀
		if err != nil || user == nil {
			continue
		}
		users = append(users, *user)
	}
	respondJSON(w, http.StatusOK, users)
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID      string `json:"user_id"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	user, err := h.redis.GetUser(r.Context(), req.UserID)
	if err != nil || user == nil {
		respondError(w, http.StatusNotFound, "User not found")
		return
	}
	user.Password = h.auth.HashPassword(req.NewPassword)
	if err := h.redis.SaveUser(r.Context(), user); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to reset password")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Password reset"})
}

func (h *Handler) ChangeUsername(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NewUsername string `json:"new_username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	userID := r.Context().Value("user_id").(string)
	user, err := h.redis.GetUser(r.Context(), userID)
	if err != nil || user == nil {
		respondError(w, http.StatusNotFound, "User not found")
		return
	}
	user.Username = req.NewUsername
	if err := h.redis.SaveUser(r.Context(), user); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to change username")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Username changed"})
}

func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(h.config.MaxUploadSize)
	file, header, err := r.FormFile("image")
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid file")
		return
	}
	defer file.Close()

	// 计算 MD5
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to process file")
		return
	}
	file.Seek(0, 0)
	md5Sum := hex.EncodeToString(hash.Sum(nil))

	// 检查是否已存在
	if img, err := h.redis.GetImage(r.Context(), md5Sum); err == nil && img != nil {
		respondJSON(w, http.StatusOK, map[string]string{"url": fmt.Sprintf("/image/%s", md5Sum)})
		return
	}

	// 验证 MIME 类型
	mimeType, err := detectMIME(file)
	if err != nil || !isImageMIME(mimeType) {
		respondError(w, http.StatusBadRequest, "Unsupported file type")
		return
	}

	// 创建临时文件
	tempFile, err := os.CreateTemp("", "upload-*")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create temp file")
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// 复制文件内容到临时文件
	file.Seek(0, 0)
	if _, err := io.Copy(tempFile, file); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save temp file")
		return
	}
	tempFile.Seek(0, 0)

	// 保存文件
	filename := md5Sum + filepath.Ext(header.Filename)
	path := h.storage.GetFilePath(filename)
	if err := h.storage.SaveFile(tempFile, path); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	// 保存元数据
	userID := r.Context().Value("user_id").(string)
	isPrivate := r.FormValue("is_private") == "true"
	tags := strings.Split(r.FormValue("tags"), ",")
	img := &model.Image{
		ID:          md5Sum,
		UserID:      userID,
		Filename:    filename,
		Description: r.FormValue("description"),
		Tags:        tags,
		IsPrivate:   isPrivate,
		CreatedAt:   time.Now(),
	}
	if err := h.redis.SaveImage(r.Context(), img); err != nil {
		h.storage.DeleteFile(path)
		respondError(w, http.StatusInternalServerError, "Failed to save metadata")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"url": fmt.Sprintf("/image/%s", md5Sum)})
}

func (h *Handler) BatchUploadImages(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(h.config.MaxUploadSize)
	files := r.MultipartForm.File["images"]
	urls := []string{}

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid file")
			return
		}
		defer file.Close()

		// 计算 MD5
		hash := md5.New()
		if _, err := io.Copy(hash, file); err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to process file")
			return
		}
		file.Seek(0, 0)
		md5Sum := hex.EncodeToString(hash.Sum(nil))

		// 检查是否已存在
		if img, err := h.redis.GetImage(r.Context(), md5Sum); err == nil && img != nil {
			urls = append(urls, fmt.Sprintf("/image/%s", md5Sum))
			continue
		}

		// 验证 MIME 类型
		mimeType, err := detectMIME(file)
		if err != nil || !isImageMIME(mimeType) {
			respondError(w, http.StatusBadRequest, "Unsupported file type")
			return
		}

		// 保存文件
		filename := md5Sum + filepath.Ext(fileHeader.Filename)
		path := h.storage.GetFilePath(filename)
		if err := h.storage.SaveFile(file, path); err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to save file")
			return
		}

		// 保存元数据
		userID := r.Context().Value("user_id").(string)
		isPrivate := r.FormValue("is_private") == "true"
		img := &model.Image{
			ID:          md5Sum,
			UserID:      userID,
			Filename:    filename,
			Description: r.FormValue("description"),
			Tags:        r.Form["tags"],
			IsPrivate:   isPrivate,
			CreatedAt:   time.Now(),
		}
		if err := h.redis.SaveImage(r.Context(), img); err != nil {
			h.storage.DeleteFile(path)
			respondError(w, http.StatusInternalServerError, "Failed to save metadata")
			return
		}
		urls = append(urls, fmt.Sprintf("/image/%s", md5Sum))
	}

	respondJSON(w, http.StatusOK, map[string][]string{"urls": urls})
}

func (h *Handler) GetImage(w http.ResponseWriter, r *http.Request) {
	imageID := chi.URLParam(r, "id")
	img, err := h.redis.GetImage(r.Context(), imageID)
	if err != nil || img == nil {
		respondError(w, http.StatusNotFound, "Image not found")
		return
	}

	if img.IsPrivate {
		userID := r.Context().Value("user_id")
		if userID == nil || userID.(string) != img.UserID {
			respondError(w, http.StatusForbidden, "Private image")
			return
		}
	}

	// 增加访问计数
	if err := h.redis.IncrementView(r.Context(), imageID); err != nil {
		slog.Error("Failed to increment view", "image_id", imageID, "error", err)
	}

	// 从 Top10 缓存或文件系统读取
	path := h.storage.GetFilePath(img.Filename)
	http.ServeFile(w, r, path)
}

func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	imageID := chi.URLParam(r, "id")
	img, err := h.redis.GetImage(r.Context(), imageID)
	if err != nil || img == nil {
		respondError(w, http.StatusNotFound, "Image not found")
		return
	}

	userID := r.Context().Value("user_id").(string)
	isAdmin := r.Context().Value("is_admin").(bool)
	if img.UserID != userID && !isAdmin {
		respondError(w, http.StatusForbidden, "Unauthorized")
		return
	}

	// 删除文件和元数据
	path := h.storage.GetFilePath(img.Filename)
	if err := h.storage.DeleteFile(path); err != nil {
		slog.Error("Failed to delete file", "path", path, "error", err)
	}
	if err := h.redis.Del(r.Context(), fmt.Sprintf("image:%s", imageID)).Err(); err != nil {
		slog.Error("Failed to delete metadata", "image_id", imageID, "error", err)
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Image deleted"})
}

func (h *Handler) BatchDeleteImages(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	userID := r.Context().Value("user_id").(string)
	isAdmin := r.Context().Value("is_admin").(bool)

	for _, imageID := range req.IDs {
		img, err := h.redis.GetImage(r.Context(), imageID)
		if err != nil || img == nil {
			continue
		}
		if img.UserID != userID && !isAdmin {
			continue
		}

		path := h.storage.GetFilePath(img.Filename)
		if err := h.storage.DeleteFile(path); err != nil {
			slog.Error("Failed to delete file", "path", path, "error", err)
		}
		if err := h.redis.Del(r.Context(), fmt.Sprintf("image:%s", imageID)).Err(); err != nil {
			slog.Error("Failed to delete metadata", "image_id", imageID, "error", err)
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Images deleted"})
}

func (h *Handler) SearchImages(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 10
	}

	images, err := h.redis.SearchImages(r.Context(), query, offset, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to search images")
		return
	}

	userID := r.Context().Value("user_id")
	isAdmin := r.Context().Value("is_admin")
	filtered := []*model.Image{}
	for _, img := range images {
		if img.IsPrivate && (userID == nil || (img.UserID != userID.(string) && !isAdmin.(bool))) {
			continue
		}
		filtered = append(filtered, img)
	}

	respondJSON(w, http.StatusOK, filtered)
}

func detectMIME(file multipart.File) (string, error) {
	buf := make([]byte, 512)
	_, err := file.Read(buf)
	if err != nil {
		return "", err
	}
	file.Seek(0, 0)
	return http.DetectContentType(buf), nil
}

func isImageMIME(mime string) bool {
	return mime == "image/jpeg" || mime == "image/png" || mime == "image/gif" || mime == "image/webp"
}

func respondError(w http.ResponseWriter, status int, message string) {
	slog.Error("Request failed", "status", status, "message", message)
	respondJSON(w, status, map[string]string{"error": message})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
