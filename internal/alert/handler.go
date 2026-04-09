package alert

import (
	"net/http"
	"strconv"

	"iot-platform/pkg/models"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	manager *Manager
}

func NewHandler(manager *Manager) *Handler {
	return &Handler{manager: manager}
}

func (h *Handler) ListRules(c *gin.Context) {
	userID := c.GetUint("user_id")

	rules, err := h.manager.ListRules(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rules": rules,
		"total": len(rules),
		"max":   MaxRulesPerUser,
	})
}

func (h *Handler) CreateRule(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req models.AlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule, err := h.manager.CreateRule(userID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, rule)
}

func (h *Handler) GetRule(c *gin.Context) {
	userID := c.GetUint("user_id")
	ruleID := c.Param("id")

	rule, err := h.manager.GetRule(ruleID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}

	c.JSON(http.StatusOK, rule)
}

func (h *Handler) UpdateRule(c *gin.Context) {
	userID := c.GetUint("user_id")
	ruleID := c.Param("id")

	var req models.AlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule, err := h.manager.UpdateRule(ruleID, userID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rule)
}

func (h *Handler) DeleteRule(c *gin.Context) {
	userID := c.GetUint("user_id")
	ruleID := c.Param("id")

	if err := h.manager.DeleteRule(ruleID, userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rule deleted"})
}

func (h *Handler) EnableRule(c *gin.Context) {
	userID := c.GetUint("user_id")
	ruleID := c.Param("id")

	if err := h.manager.EnableRule(ruleID, userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rule enabled"})
}

func (h *Handler) DisableRule(c *gin.Context) {
	userID := c.GetUint("user_id")
	ruleID := c.Param("id")

	if err := h.manager.DisableRule(ruleID, userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rule disabled"})
}

func (h *Handler) ListAlerts(c *gin.Context) {
	userID := c.GetUint("user_id")
	status := c.Query("status")
	limitStr := c.DefaultQuery("limit", "20")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	alerts, total, err := h.manager.GetAlerts(userID, status, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"alerts": alerts,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) AcknowledgeAlert(c *gin.Context) {
	userID := c.GetUint("user_id")
	alertID := c.Param("id")

	if err := h.manager.AcknowledgeAlert(alertID, userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "alert acknowledged"})
}

func (h *Handler) ResolveAlert(c *gin.Context) {
	userID := c.GetUint("user_id")
	alertID := c.Param("id")

	if err := h.manager.ResolveAlert(alertID, userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "alert resolved"})
}

func (h *Handler) GetAlertStats(c *gin.Context) {
	userID := c.GetUint("user_id")

	stats, err := h.manager.GetAlertStats(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}
