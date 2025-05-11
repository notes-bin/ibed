# 项目简介
本项目 github.com/notes-bin/ibed 是一个基于Go语言开发的图片管理系统，支持用户注册、登录、图片上传、删除、搜索等功能，同时使用Redis进行数据管理和缓存，以提高系统性能。

## 功能概述
### 用户管理
- 注册 ：用户可以使用用户名和密码进行注册，首次注册 admin 用户将成为超级管理员。
- 登录 ：支持用户使用用户名和密码登录系统，并生成JWT令牌用于身份验证。
- 修改密码 ：已登录用户可以修改自己的密码。
- 删除用户 ：用户可以删除自己的账户。
- 管理员操作 ：超级管理员可以查看所有用户列表、重置用户密码和修改用户名。
### 图片管理
- 上传图片 ：支持单张和批量图片上传，上传时可设置图片描述和标签。
- 删除图片 ：支持单张和批量图片删除。
- 搜索图片 ：用户可以根据图片描述和标签搜索图片。
- 访问图片 ：支持公有和私有图片访问，私有图片需要用户登录后才能访问。
### 缓存机制
- Top10缓存 ：定期从Redis获取访问次数最高的10张图片并更新缓存，提高热门图片的访问速度。
## 使用方法
### 环境准备
- 安装Go 1.23.2及以上版本。
- 安装Redis，并确保Redis服务正常运行。
- 安装项目依赖：
```bash
go mod tidy
```
### 配置文件
在 config/config.json 文件中配置项目参数，示例如下：
```json
{
    "Redis": {
        "addr": "localhost:6379",
        "password": "",
        "db": 0,
        "pool_size": 10
    },
    "JWTSecret": "your_jwt_secret",
    "UploadDir": "./uploads",
    "RateLimit": {
        "Requests": 100,
        "Duration": 60
    },
    "Port": "8080",
    "TopRefreshInterval": 300
}
```
### 启动项目
```bash
go run main.go
```
## API 文档
### 用户相关
- POST /register 注册用户。
  
  - Body: { "username": "string", "password": "string" }
  - Response: { "message": "User registered", "user_id": "string" }
- POST /login 用户登录，指定 token 过期时间（秒）。
  
  - Body: { "username": "string", "password": "string", "expires_in": int }
  - Response: { "token": "string" }
- POST /change-password 修改密码，管理员首次登录需调用。
  
  - Header: Authorization: Bearer
  - Body: { "old_password": "string", "new_password": "string" }
  - Response: { "message": "Password changed" }
- POST /refresh-token 延长 token 有效期。
  
  - Header: Authorization: Bearer
  - Body: { "expires_in": int }
  - Response: { "token": "string" }
- DELETE /user 注销用户。
  
  - Header: Authorization: Bearer
  - Response: { "message": "User deleted" }
- GET /users (管理员)列出所有用户。
  
  - Header: Authorization: Bearer
  - Response: [ { "id": "string", "username": "string", "is_admin": bool } ]
- POST /reset-password (管理员)重置用户密码。
  
  - Header: Authorization: Bearer
  - Body: { "user_id": "string", "new_password": "string" }
  - Response: { "message": "Password reset" }
- POST /change-username (管理员)修改用户名。
  
  - Header: Authorization: Bearer
  - Body: { "new_username": "string" }
  - Response: { "message": "Username changed" }
### 图片相关
- POST /upload 上传图片。
  
  - Header: Authorization: Bearer
  - Form: image (文件), description (string), tags (array), is_private (bool)
  - Response: { "url": "string" }
- POST /batch-upload 批量上传图片。
  
  - Header: Authorization: Bearer
  - Form: images (多文件), description (string), tags (array), is_private (bool)
  - Response: { "urls": ["string"] }
- GET /image/{id} 获取图片。
  
  - Header: Authorization: Bearer
    (私有图片)
  - Response: 图片文件
- DELETE /image/{id} 删除图片。
  
  - Header: Authorization: Bearer
  - Response: { "message": "Image deleted" }
- POST /batch-delete 批量删除图片。
  
  - Header: Authorization: Bearer
  - Body: { "ids": ["string"] }
  - Response: { "message": "Images deleted" }
- GET /search 搜索图片（支持标签和描述）。
  
  - Query: q (string), offset (int), limit (int)
  - Header: Authorization: Bearer
    (私有图片)
  - Response: [ { "id": "string", "url": "string", "description": "string", "tags": ["string"] } ]
## 常见问题
### 1. 如何设置管理员账户？
首次注册时，用户名为 "admin" 的账户将自动成为超级管理员。

### 2. 如何修改密码？
已登录用户可以通过 /change-password 接口修改密码，无需输入原密码。

### 3. 如何删除用户？
普通用户可以通过 /user 接口删除自己的账户，管理员可以通过该接口删除其他用户。

### 4. 如何上传图片？
使用 /upload 接口上传图片，支持设置图片描述、标签和是否为私有图片。

### 5. 如何搜索图片？
使用 /search 接口搜索图片，支持根据描述和标签进行模糊匹配。

## 完整API使用示例

### 1. 注册用户
```bash
curl -X POST http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"test123"}'
```
### 2. 用户登录
```bash
# 登录并获取token，设置token有效期为1小时(3600秒)
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"test123","expires_in":3600}'

# 响应示例
# {"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."}
```
### 3. 上传图片
```bash
# 使用获取的token上传图片
curl -X POST http://localhost:8080/upload \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -F "image=@example.jpg" \
  -F "description=示例图片" \
  -F "tags=scenery" \
  -F "tags=travel" \
  -F "is_private=false"

# 响应示例
# {"url":"/image/d41d8cd98f00b204e9800998ecf8427e"}
```
### 4. 批量上传图片
```bash
curl -X POST http://localhost:8080/batch-upload \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -F "images=@image1.jpg" \
  -F "images=@image2.jpg" \
  -F "description=批量上传示例" \
  -F "tags=batch" \
  -F "is_private=true"
  ```
  ### 5. 搜索图片
  ```bash
curl -X GET "http://localhost:8080/search?q=示例&offset=0&limit=10" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  ```
### 6. 获取图片
```bash
# 公开图片
curl http://localhost:8080/image/d41d8cd98f00b204e9800998ecf8427e -o downloaded.jpg

# 私有图片需要token
curl -X GET http://localhost:8080/image/private_image_id \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -o private_image.jpg
  ```
### 7. 修改密码
```bash
curl -X POST http://localhost:8080/change-password \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -H "Content-Type: application/json" \
  -d '{"new_password":"newpassword123"}'
  ```

  ### 8. 管理员操作示例
  ```bash
  # 列出所有用户
curl -X GET http://localhost:8080/users \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# 重置用户密码
curl -X POST http://localhost:8080/reset-password \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -H "Content-Type: application/json" \
  -d '{"user_id":"target_user","new_password":"newpass123"}'
  ```

## 部署与运行
1. 安装 Redis（确保运行，建议安装 RediSearch 模块以优化搜索）。
2. 创建 config/config.json，填写配置。
3. 运行 go mod tidy 下载依赖。
4. 运行 go run main.go 启动服务。
5. 使用 Postman 或 curl 测试 API。