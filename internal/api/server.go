package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"iot-platform/internal/config"
	"iot-platform/internal/device"
	"iot-platform/internal/mqtt"
	"iot-platform/internal/storage"
	"iot-platform/internal/websocket"
	"iot-platform/pkg/models"

	"github.com/gin-gonic/gin"
)

type Server struct {
	router     *gin.Engine
	config     *config.ServerConfig
	deviceMgr  *device.Manager
	mqttServer *mqtt.Server
	store      *storage.Store
	wsHub      *websocket.Hub
}

func NewServer(cfg *config.ServerConfig, deviceMgr *device.Manager, mqttServer *mqtt.Server, store *storage.Store, wsHub *websocket.Hub) *Server {
	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	s := &Server{
		router:     r,
		config:     cfg,
		deviceMgr:  deviceMgr,
		mqttServer: mqttServer,
		store:      store,
		wsHub:      wsHub,
	}

	s.setupRoutes()

	return s
}

func (s *Server) setupRoutes() {
	s.router.Static("/web", "./web")
	s.router.GET("/ws", func(c *gin.Context) {
		s.wsHub.HandleWS(c.Writer, c.Request)
	})

	api := s.router.Group("/api/v1")
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
			deviceTypes.POST("", s.createDeviceType)
			deviceTypes.PUT("/:id", s.updateDeviceType)
			deviceTypes.DELETE("/:id", s.deleteDeviceType)
		}

		api.GET("/stats", s.getStats)
	}

	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.config.HTTPAddr)
	return s.router.Run(addr)
}

type RegisterDeviceRequest struct {
	Name       string                 `json:"name" binding:"required"`
	Type       string                 `json:"type" binding:"required"`
	Properties map[string]interface{} `json:"properties"`
}

func (s *Server) registerDevice(c *gin.Context) {
	var req RegisterDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, err := s.deviceMgr.Register(req.Name, req.Type, req.Properties)
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
	devices, err := s.store.ListDevicesWithTypes()
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

	device, err := s.store.GetDeviceWithType(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	c.JSON(http.StatusOK, device)
}

func (s *Server) deleteDevice(c *gin.Context) {
	id := c.Param("id")

	device, err := s.store.GetDeviceWithType(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
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
	Name string `json:"name"`
	Type string `json:"type"`
}

func (s *Server) updateDevice(c *gin.Context) {
	id := c.Param("id")

	var req UpdateDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.deviceMgr.UpdateDeviceInfo(id, req.Name, req.Type); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "device updated"})
}

func (s *Server) updateProperties(c *gin.Context) {
	id := c.Param("id")

	var props map[string]interface{}
	if err := c.ShouldBindJSON(&props); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var name, deviceType string
	if n, ok := props["name"].(string); ok {
		name = n
		delete(props, "name")
	}
	if t, ok := props["type"].(string); ok {
		deviceType = t
		delete(props, "type")
	}

	if name != "" || deviceType != "" {
		if err := s.deviceMgr.UpdateDeviceInfo(id, name, deviceType); err != nil {
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

type SendCommandRequest struct {
	Command string                 `json:"command" binding:"required"`
	Params  map[string]interface{} `json:"params"`
}

func (s *Server) sendCommand(c *gin.Context) {
	id := c.Param("id")

	var req SendCommandRequest
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

func (s *Server) disableDevice(c *gin.Context) {
	id := c.Param("id")

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
