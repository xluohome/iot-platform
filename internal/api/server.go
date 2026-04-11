package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"iot-platform/internal/alert"
	"iot-platform/internal/auth"
	"iot-platform/internal/config"
	"iot-platform/internal/device"
	"iot-platform/internal/firmware"
	"iot-platform/internal/mqtt"
	"iot-platform/internal/storage"
	"iot-platform/internal/websocket"
	"iot-platform/pkg/models"

	"github.com/gin-gonic/gin"
)

type Server struct {
	router          *gin.Engine
	config          *config.Config
	deviceMgr       *device.Manager
	mqttServer      *mqtt.Server
	store           *storage.Store
	wsHub           *websocket.Hub
	jwtManager      *auth.JWTManager
	authHandler     *auth.AuthHandler
	authMiddleware  *auth.AuthMiddleware
	alertHandler    *alert.Handler
	firmwareHandler *firmware.Handler
}

func NewServer(cfg *config.Config, deviceMgr *device.Manager, mqttServer *mqtt.Server, store *storage.Store, wsHub *websocket.Hub, alertHandler *alert.Handler, firmwareHandler *firmware.Handler) *Server {
	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Refresh-Token")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.AccessTokenExpire, cfg.Auth.RefreshTokenExpire)
	authHandler := auth.NewAuthHandler(jwtManager, store)
	authMiddleware := auth.NewAuthMiddleware(jwtManager, store)

	s := &Server{
		router:          r,
		config:          cfg,
		deviceMgr:       deviceMgr,
		mqttServer:      mqttServer,
		store:           store,
		wsHub:           wsHub,
		jwtManager:      jwtManager,
		authHandler:     authHandler,
		authMiddleware:  authMiddleware,
		alertHandler:    alertHandler,
		firmwareHandler: firmwareHandler,
	}

	s.setupRoutes()

	return s
}

func (s *Server) setupRoutes() {
	s.router.Static("/web", "./web")
	s.router.GET("/ws", func(c *gin.Context) {
		s.wsHub.HandleWS(c.Writer, c.Request)
	})

	authGroup := s.router.Group("/api/v1/auth")
	{
		authGroup.POST("/register", s.authHandler.Register)
		authGroup.POST("/login", s.authHandler.Login)
		authGroup.POST("/refresh", s.authHandler.Refresh)
		authGroup.POST("/logout", s.authMiddleware.Authenticate(), s.authHandler.Logout)
		authGroup.GET("/me", s.authMiddleware.Authenticate(), s.authHandler.Me)
	}

	usersGroup := s.router.Group("/api/v1/users")
	usersGroup.Use(s.authMiddleware.Authenticate(), s.authMiddleware.RequireRole("admin"))
	{
		usersGroup.GET("", s.authHandler.GetUsers)
		usersGroup.POST("", s.authHandler.CreateUser)
		usersGroup.GET("/:id", s.authHandler.GetUser)
		usersGroup.PUT("/:id", s.authHandler.UpdateUser)
		usersGroup.PUT("/:id/disable", s.authHandler.DisableUser)
		usersGroup.PUT("/:id/enable", s.authHandler.EnableUser)
		usersGroup.DELETE("/:id", s.authHandler.DeleteUser)
	}

	api := s.router.Group("/api/v1")
	api.Use(s.authMiddleware.Authenticate())
	{
		devices := api.Group("/devices")
		{
			devices.GET("", s.listDevices)
			devices.POST("", s.registerDevice)
			devices.GET("/:id", s.getDevice)
			devices.DELETE("/:id", s.deleteDevice)
			devices.PUT("/:id", s.updateDevice)
			devices.PUT("/:id/properties", s.updateProperties)
			devices.POST("/:id/command", s.sendCommand)
			devices.GET("/:id/telemetry", s.getTelemetry)
			devices.GET("/:id/commands", s.getCommands)
			devices.PUT("/:id/disable", s.disableDevice)
			devices.PUT("/:id/enable", s.enableDevice)
		}

		deviceTypes := api.Group("/device-types")
		{
			deviceTypes.GET("", s.listDeviceTypes)
			deviceTypes.POST("", s.authMiddleware.RequireRole("admin"), s.createDeviceType)
			deviceTypes.PUT("/:id", s.authMiddleware.RequireRole("admin"), s.updateDeviceType)
			deviceTypes.DELETE("/:id", s.authMiddleware.RequireRole("admin"), s.deleteDeviceType)
		}

		api.GET("/stats", s.getStats)

		alerts := api.Group("/alerts")
		{
			alerts.GET("", s.alertHandler.ListAlerts)
			alerts.GET("/stats", s.alertHandler.GetAlertStats)
			alerts.PUT("/:id/acknowledge", s.alertHandler.AcknowledgeAlert)
			alerts.PUT("/:id/resolve", s.alertHandler.ResolveAlert)
		}

		alertRules := api.Group("/alert-rules")
		{
			alertRules.GET("", s.alertHandler.ListRules)
			alertRules.POST("", s.alertHandler.CreateRule)
			alertRules.GET("/:id", s.alertHandler.GetRule)
			alertRules.PUT("/:id", s.alertHandler.UpdateRule)
			alertRules.DELETE("/:id", s.alertHandler.DeleteRule)
			alertRules.PUT("/:id/enable", s.alertHandler.EnableRule)
			alertRules.PUT("/:id/disable", s.alertHandler.DisableRule)
		}

		firmwares := api.Group("/firmwares")
		{
			firmwares.GET("", s.firmwareHandler.ListFirmwares)
			firmwares.GET("/:id", s.firmwareHandler.GetFirmware)
			firmwares.POST("", s.firmwareHandler.UploadFirmware)
			firmwares.DELETE("/:id", s.firmwareHandler.DeleteFirmware)
		}

		api.GET("/firmwares/:id/download", s.firmwareHandler.DownloadFirmware)

		api.GET("/devices/:id/firmware", s.firmwareHandler.GetDeviceFirmware)
		api.POST("/devices/:id/upgrade", s.firmwareHandler.UpgradeDevice)
		api.GET("/devices/:id/upgrade-status", s.firmwareHandler.GetUpgradeStatus)
		api.POST("/devices/upgrade", s.firmwareHandler.BatchUpgradeDevices)

		api.GET("/upgrade-tasks", s.firmwareHandler.ListUpgradeTasks)
		api.GET("/upgrade-tasks/:id", s.firmwareHandler.GetUpgradeTask)
		api.POST("/upgrade-tasks/:id/expand", s.firmwareHandler.ExpandTask)
		api.POST("/upgrade-tasks/:id/cancel", s.firmwareHandler.CancelTask)
		api.POST("/upgrade-tasks/:id/retry", s.firmwareHandler.RetryFailed)
	}

	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.config.Server.HTTPAddr)
	return s.router.Run(addr)
}

type RegisterDeviceRequest struct {
	Name       string                 `json:"name" binding:"required"`
	Type       string                 `json:"type" binding:"required"`
	UserID     uint                   `json:"user_id"`
	Properties map[string]interface{} `json:"properties"`
}

func (s *Server) registerDevice(c *gin.Context) {
	var req RegisterDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	currentUserID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	var deviceUserID uint
	if role.(string) == "admin" && req.UserID > 0 {
		deviceUserID = req.UserID
	} else {
		deviceUserID = currentUserID.(uint)
	}

	device, err := s.deviceMgr.Register(req.Name, req.Type, req.Properties, deviceUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.wsHub.Broadcast(&websocket.Message{
		Type:    "device_registered",
		Payload: device,
	})

	c.JSON(http.StatusCreated, device)
}

func (s *Server) listDevices(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	var devices []*models.DeviceResponse
	var err error

	if role.(string) == "admin" {
		devices, err = s.store.ListDevicesWithTypes()
	} else {
		devices, err = s.store.ListDevicesByUserID(userID.(uint))
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"devices": devices,
		"total":   len(devices),
	})
}

func (s *Server) getDevice(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	device, err := s.store.GetDeviceWithTypeAndUser(id, userID.(uint), role.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	c.JSON(http.StatusOK, device)
}

func (s *Server) deleteDevice(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	device, err := s.store.GetDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	if role.(string) != "admin" && device.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if device.Status == "online" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete online device, disable it first"})
		return
	}

	if err := s.deviceMgr.Unregister(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.wsHub.Broadcast(&websocket.Message{
		Type:    "device_unregistered",
		Payload: map[string]string{"id": id},
	})

	c.JSON(http.StatusOK, gin.H{"message": "device deleted"})
}

type UpdateDeviceRequest struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	UserID uint   `json:"user_id"`
}

func (s *Server) updateDevice(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	device, err := s.store.GetDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	if role.(string) != "admin" && device.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req UpdateDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.deviceMgr.UpdateDeviceInfo(id, req.Name, req.Type); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if role.(string) == "admin" && req.UserID > 0 {
		if err := s.deviceMgr.UpdateDeviceOwner(id, req.UserID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "device updated"})
}

func (s *Server) updateProperties(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	device, err := s.store.GetDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	if role.(string) != "admin" && device.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var props map[string]interface{}
	if err := c.ShouldBindJSON(&props); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var name, deviceType string
	var newUserID uint
	var hasUserID bool
	if n, ok := props["name"].(string); ok {
		name = n
		delete(props, "name")
	}
	if t, ok := props["type"].(string); ok {
		deviceType = t
		delete(props, "type")
	}
	if uid, ok := props["user_id"].(float64); ok && role.(string) == "admin" {
		newUserID = uint(uid)
		hasUserID = true
		delete(props, "user_id")
	}

	if name != "" || deviceType != "" {
		if err := s.deviceMgr.UpdateDeviceInfo(id, name, deviceType); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if hasUserID {
		if err := s.deviceMgr.UpdateDeviceOwner(id, newUserID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if err := s.deviceMgr.UpdateProperties(id, props); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "properties updated"})
}

func (s *Server) sendCommand(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	device, err := s.store.GetDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	if role.(string) != "admin" && device.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req struct {
		Command string                 `json:"command" binding:"required"`
		Params  map[string]interface{} `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	params, _ := json.Marshal(req.Params)
	record, err := s.store.SaveCommand(id, req.Command, string(params))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := s.mqttServer.PublishCommand(id, fmt.Sprintf("%d", record.ID), req.Command, req.Params); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"command_id": record.ID,
		"status":     "pending",
	})
}

func (s *Server) getTelemetry(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	device, err := s.store.GetDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	if role.(string) != "admin" && device.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	records, err := s.store.GetTelemetry(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"device_id": id,
		"telemetry": records,
		"count":     len(records),
	})
}

func (s *Server) getCommands(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	device, err := s.store.GetDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	if role.(string) != "admin" && device.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)

	commands, err := s.store.GetCommands(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"device_id": id,
		"commands":  commands,
		"count":     len(commands),
	})
}

func (s *Server) disableDevice(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	device, err := s.store.GetDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	if role.(string) != "admin" && device.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := s.deviceMgr.DisableDevice(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.mqttServer.DisconnectDevice(id)

	s.wsHub.Broadcast(&websocket.Message{
		Type:    "device_updated",
		Payload: map[string]interface{}{"id": id, "disabled": true},
	})

	c.JSON(http.StatusOK, gin.H{"message": "device disabled"})
}

func (s *Server) enableDevice(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	device, err := s.store.GetDeviceByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	if role.(string) != "admin" && device.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := s.deviceMgr.EnableDevice(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.wsHub.Broadcast(&websocket.Message{
		Type:    "device_updated",
		Payload: map[string]interface{}{"id": id, "disabled": false},
	})

	c.JSON(http.StatusOK, gin.H{"message": "device enabled"})
}

type DeviceTypeResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	DeviceCount int64  `json:"device_count"`
}

func (s *Server) listDeviceTypes(c *gin.Context) {
	types, err := s.store.GetAllDeviceTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]DeviceTypeResponse, 0, len(types))
	for _, t := range types {
		count, _ := s.store.GetDeviceCountByType(t.ID)
		result = append(result, DeviceTypeResponse{
			ID:          t.ID,
			Name:        t.Name,
			DeviceCount: count,
		})
	}

	c.JSON(http.StatusOK, gin.H{"device_types": result})
}

func (s *Server) createDeviceType(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dt, err := s.store.CreateDeviceType(req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, dt)
}

func (s *Server) updateDeviceType(c *gin.Context) {
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.store.UpdateDeviceType(id, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "device type updated"})
}

func (s *Server) deleteDeviceType(c *gin.Context) {
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var dt models.DeviceType
	if err := s.store.DB().First(&dt, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device type not found"})
		return
	}

	count, _ := s.store.GetDeviceCountByType(dt.ID)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("该类型下有 %d 个设备，无法删除", count)})
		return
	}

	if err := s.store.DeleteDeviceType(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "device type deleted"})
}

func (s *Server) getStats(c *gin.Context) {
	stats := s.deviceMgr.GetStats()
	stats["ws_clients"] = s.wsHub.ClientCount()

	c.JSON(http.StatusOK, stats)
}
