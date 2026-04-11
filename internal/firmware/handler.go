package firmware

import (
	"fmt"
	"net/http"
	"strconv"

	"iot-platform/internal/auth"
	"iot-platform/internal/device"
	"iot-platform/pkg/models"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	manager   *Manager
	deviceMgr *device.Manager
	jwtSecret []byte
}

func NewHandler(manager *Manager, deviceMgr *device.Manager, jwtSecret []byte) *Handler {
	return &Handler{
		manager:   manager,
		deviceMgr: deviceMgr,
		jwtSecret: jwtSecret,
	}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/firmwares", h.ListFirmwares)
	r.GET("/firmwares/:id", h.GetFirmware)
	r.POST("/firmwares", h.UploadFirmware)
	r.DELETE("/firmwares/:id", h.DeleteFirmware)
	r.GET("/firmwares/:id/download", h.DownloadFirmware)

	r.GET("/devices/:id/firmware", h.GetDeviceFirmware)
	r.POST("/devices/:id/upgrade", h.UpgradeDevice)
	r.GET("/devices/:id/upgrade-status", h.GetUpgradeStatus)

	r.POST("/devices/upgrade", h.BatchUpgradeDevices)

	r.GET("/upgrade-tasks", h.ListUpgradeTasks)
	r.GET("/upgrade-tasks/:id", h.GetUpgradeTask)
}

func (h *Handler) ListFirmwares(c *gin.Context) {
	deviceType := c.Query("device_type")

	firmwares, err := h.manager.ListFirmwares(deviceType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"firmwares": firmwares,
		"total":     len(firmwares),
	})
}

func (h *Handler) GetFirmware(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid firmware id"})
		return
	}

	fw, err := h.manager.GetFirmware(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "firmware not found"})
		return
	}

	c.JSON(http.StatusOK, fw)
}

func (h *Handler) UploadFirmware(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	name := c.PostForm("name")
	version := c.PostForm("version")
	deviceType := c.PostForm("device_type")
	description := c.PostForm("description")

	if name == "" || version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and version are required"})
		return
	}

	fw, err := h.manager.UploadFirmware(name, version, deviceType, description, file, header.Size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, fw)
}

func (h *Handler) DeleteFirmware(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid firmware id"})
		return
	}

	if err := h.manager.DeleteFirmware(uint(id)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "firmware not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "firmware deleted"})
}

func (h *Handler) DownloadFirmware(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid firmware id"})
		return
	}

	fw, err := h.manager.GetFirmware(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "firmware not found"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s-%s.bin", fw.Name, fw.Version))
	c.Header("Content-Type", "application/octet-stream")
	c.File(fw.FilePath)
}

func (h *Handler) GetDeviceFirmware(c *gin.Context) {
	deviceID := c.Param("id")

	df, err := h.manager.GetDeviceFirmware(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"device_id": deviceID,
			"version":   "",
			"firmware":  nil,
		})
		return
	}

	fw, _ := h.manager.GetFirmware(df.FirmwareID)
	c.JSON(http.StatusOK, gin.H{
		"device_id": df.DeviceID,
		"version":   df.Version,
		"firmware":  fw,
	})
}

func (h *Handler) UpgradeDevice(c *gin.Context) {
	deviceID := c.Param("id")

	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(uint)

	var req struct {
		FirmwareID uint `json:"firmware_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	d, err := h.deviceMgr.GetDevice(deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	if d.UserID != userID && !h.isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return
	}

	taskDevice, err := h.manager.UpgradeDevice(deviceID, req.FirmwareID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, taskDevice)
}

func (h *Handler) GetUpgradeStatus(c *gin.Context) {
	deviceID := c.Param("id")

	status, err := h.manager.GetUpgradeStatus(deviceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": "idle"})
		return
	}

	c.JSON(http.StatusOK, status)
}

func (h *Handler) BatchUpgradeDevices(c *gin.Context) {
	var req models.UpgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	percentage := req.Percentage
	if percentage <= 0 || percentage > 100 {
		percentage = 100
	}

	task, err := h.manager.CreateUpgradeTaskByPercentage(req.FirmwareID, percentage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

func (h *Handler) ListUpgradeTasks(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	tasks, total, err := h.manager.ListUpgradeTasks(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":  tasks,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) GetUpgradeTask(c *gin.Context) {
	taskID := c.Param("id")

	task, devices, err := h.manager.GetUpgradeTask(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task":    task,
		"devices": devices,
	})
}

func (h *Handler) isAdmin(c *gin.Context) bool {
	claims, _ := c.Get("claims")
	return claims.(*auth.Claims).Role == "admin"
}

func (h *Handler) ServeFirmwareDownload(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid firmware id"})
		return
	}

	fw, err := h.manager.GetFirmware(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "firmware not found"})
		return
	}

	c.File(fw.FilePath)
}

func (h *Handler) ExpandTask(c *gin.Context) {
	taskID := c.Param("id")

	var req models.ExpandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Percentage <= 0 || req.Percentage > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "percentage must be between 1 and 100"})
		return
	}

	if err := h.manager.ExpandTask(taskID, req.Percentage); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, devices, err := h.manager.GetUpgradeTask(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task":    task,
		"devices": devices,
	})
}

func (h *Handler) CancelTask(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.manager.CancelTask(taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "task cancelled"})
}

func (h *Handler) RetryFailed(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.manager.RetryFailedDevices(taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, devices, err := h.manager.GetUpgradeTask(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task":    task,
		"devices": devices,
	})
}
