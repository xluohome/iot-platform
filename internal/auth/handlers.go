package auth

import (
	"fmt"
	"net/http"
	"time"

	"iot-platform/internal/storage"
	"iot-platform/pkg/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AuthHandler struct {
	jwtManager *JWTManager
	store      *storage.Store
}

func NewAuthHandler(jwtManager *JWTManager, store *storage.Store) *AuthHandler {
	return &AuthHandler{
		jwtManager: jwtManager,
		store:      store,
	}
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, _ := h.store.GetUserByUsername(req.Username)
	if existing != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username already exists"})
		return
	}

	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user := &models.User{
		Username:     req.Username,
		PasswordHash: passwordHash,
		Role:         "user",
	}

	if err := h.store.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":       user.ID,
		"username": user.Username,
		"role":     user.Role,
	})
}

type CreateUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role"`
}

func (h *AuthHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, _ := h.store.GetUserByUsername(req.Username)
	if existing != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username already exists"})
		return
	}

	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	role := "user"
	if req.Role == "admin" {
		role = "admin"
	}

	user := &models.User{
		Username:     req.Username,
		PasswordHash: passwordHash,
		Role:         role,
	}

	if err := h.store.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":       user.ID,
		"username": user.Username,
		"role":     user.Role,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.store.GetUserByUsername(req.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if user.Disabled {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户已被禁用"})
		return
	}

	if !CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	accessToken, err := h.jwtManager.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate access token"})
		return
	}

	refreshToken, expiresAt, err := h.jwtManager.GenerateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate refresh token"})
		return
	}

	refreshTokenRecord := &models.RefreshToken{
		UserID:    user.ID,
		Token:     refreshToken,
		ExpiresAt: expiresAt,
	}
	if err := h.store.SaveRefreshToken(refreshTokenRecord); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    int64(h.jwtManager.GetAccessTokenTTL().Seconds()),
		"token_type":    "Bearer",
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.jwtManager.ValidateRefreshToken(req.RefreshToken); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	tokenRecord, err := h.store.GetRefreshToken(req.RefreshToken)
	if err != nil || tokenRecord == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token not found"})
		return
	}

	if tokenRecord.ExpiresAt.Before(time.Now()) {
		h.store.DeleteRefreshToken(req.RefreshToken)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token expired"})
		return
	}

	user, err := h.store.GetUserByID(tokenRecord.UserID)
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	h.store.DeleteRefreshToken(req.RefreshToken)

	accessToken, err := h.jwtManager.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate access token"})
		return
	}

	refreshToken, expiresAt, err := h.jwtManager.GenerateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate refresh token"})
		return
	}

	refreshTokenRecord := &models.RefreshToken{
		UserID:    user.ID,
		Token:     refreshToken,
		ExpiresAt: expiresAt,
	}
	if err := h.store.SaveRefreshToken(refreshTokenRecord); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"expires_in":   int64(h.jwtManager.GetAccessTokenTTL().Seconds()),
		"token_type":   "Bearer",
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	refreshToken := c.GetHeader("X-Refresh-Token")
	if refreshToken != "" {
		h.store.DeleteRefreshToken(refreshToken)
	}

	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	user, err := h.store.GetUserByID(userID.(uint))
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"role":       user.Role,
		"disabled":   user.Disabled,
		"created_at": user.CreatedAt,
	})
}

type UpdateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *AuthHandler) GetUsers(c *gin.Context) {
	users, err := h.store.GetAllUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get users"})
		return
	}

	result := make([]gin.H, 0, len(users))
	for _, u := range users {
		result = append(result, gin.H{
			"id":         u.ID,
			"username":   u.Username,
			"role":       u.Role,
			"disabled":   u.Disabled,
			"created_at": u.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"users": result})
}

func (h *AuthHandler) GetUser(c *gin.Context) {
	id := c.Param("id")

	userID, err := parseUint(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	user, err := h.store.GetUserByID(userID)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"role":       user.Role,
		"disabled":   user.Disabled,
		"created_at": user.CreatedAt,
	})
}

func (h *AuthHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")

	userID, err := parseUint(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.store.GetUserByID(userID)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	updates := make(map[string]interface{})

	if req.Username != "" {
		existing, _ := h.store.GetUserByUsername(req.Username)
		if existing != nil && existing.ID != user.ID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "username already exists"})
			return
		}
		updates["username"] = req.Username
	}

	if req.Password != "" {
		passwordHash, err := HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}
		updates["password_hash"] = passwordHash
	}

	if req.Role != "" {
		if req.Role != "admin" && req.Role != "user" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
			return
		}
		updates["role"] = req.Role
	}

	if len(updates) > 0 {
		if err := h.store.UpdateUser(userID, updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
			return
		}
	}

	user, _ = h.store.GetUserByID(userID)
	c.JSON(http.StatusOK, gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"role":       user.Role,
		"disabled":   user.Disabled,
		"created_at": user.CreatedAt,
	})
}

func (h *AuthHandler) DisableUser(c *gin.Context) {
	id := c.Param("id")

	userID, err := parseUint(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	currentUserID, _ := c.Get("user_id")
	if currentUserID.(uint) == userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能禁用当前登录用户"})
		return
	}

	user, err := h.store.GetUserByID(userID)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := h.store.DisableUser(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to disable user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "用户已禁用"})
}

func (h *AuthHandler) EnableUser(c *gin.Context) {
	id := c.Param("id")

	userID, err := parseUint(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	user, err := h.store.GetUserByID(userID)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := h.store.EnableUser(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enable user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "用户已启用"})
}

func (h *AuthHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")

	userID, err := parseUint(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	currentUserID, _ := c.Get("user_id")
	if currentUserID.(uint) == userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete yourself"})
		return
	}

	user, err := h.store.GetUserByID(userID)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	deviceCount, _ := h.store.GetDeviceCountByUserID(userID)
	if deviceCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("该用户拥有 %d 个设备，无法删除", deviceCount)})
		return
	}

	if err := h.store.DeleteUser(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

func parseUint(s string) (uint, error) {
	var id uint
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		id = id*10 + uint(c-'0')
	}
	return id, nil
}

func GenerateUUID() string {
	return uuid.New().String()
}
