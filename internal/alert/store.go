package alert

import (
	"fmt"
	"time"

	"iot-platform/pkg/models"

	"gorm.io/gorm"
)

const MaxRulesPerUser = 10

type AlertStore struct {
	db *gorm.DB
}

func NewAlertStore(db *gorm.DB) *AlertStore {
	return &AlertStore{db: db}
}

func (s *AlertStore) AutoMigrate() error {
	return s.db.AutoMigrate(&models.AlertRule{}, &models.Alert{})
}

func (s *AlertStore) CountRulesByUser(userID uint) (int64, error) {
	var count int64
	err := s.db.Model(&models.AlertRule{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

func (s *AlertStore) CreateRule(rule *models.AlertRule) error {
	count, err := s.CountRulesByUser(rule.UserID)
	if err != nil {
		return err
	}
	if count >= MaxRulesPerUser {
		return fmt.Errorf("每个用户最多创建 %d 条规则", MaxRulesPerUser)
	}
	return s.db.Create(rule).Error
}

func (s *AlertStore) UpdateRule(rule *models.AlertRule) error {
	return s.db.Save(rule).Error
}

func (s *AlertStore) DeleteRule(id string) error {
	return s.db.Delete(&models.AlertRule{}, "id = ?", id).Error
}

func (s *AlertStore) GetRule(id string) (*models.AlertRule, error) {
	var rule models.AlertRule
	err := s.db.First(&rule, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *AlertStore) ListRules(userID uint) ([]*models.AlertRule, error) {
	var rules []*models.AlertRule
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (s *AlertStore) ListAllEnabledRules() ([]*models.AlertRule, error) {
	var rules []*models.AlertRule
	err := s.db.Where("enabled = ?", true).Find(&rules).Error
	return rules, err
}

func (s *AlertStore) GetRulesForDevice(deviceID string) ([]*models.AlertRule, error) {
	var rules []*models.AlertRule
	err := s.db.Where("enabled = ? AND (device_id = ? OR device_id IS NULL OR device_id = '')", true, deviceID).Find(&rules).Error
	return rules, err
}

func (s *AlertStore) GetRulesForDeviceType(deviceType string) ([]*models.AlertRule, error) {
	var rules []*models.AlertRule
	err := s.db.Where("enabled = ? AND (device_type = ? OR device_type IS NULL OR device_type = '')", true, deviceType).Find(&rules).Error
	return rules, err
}

func (s *AlertStore) UpdateLastTriggered(id string, t time.Time) error {
	return s.db.Model(&models.AlertRule{}).Where("id = ?", id).Update("last_triggered", t).Error
}

func (s *AlertStore) CreateAlert(alert *models.Alert) error {
	return s.db.Create(alert).Error
}

func (s *AlertStore) GetAlert(id string) (*models.Alert, error) {
	var alert models.Alert
	err := s.db.First(&alert, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &alert, nil
}

func (s *AlertStore) ListAlerts(userID uint, status string, limit, offset int) ([]*models.Alert, int64, error) {
	var alerts []*models.Alert
	var total int64

	query := s.db.Model(&models.Alert{}).Where("user_id = ?", userID)
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&alerts).Error
	return alerts, total, err
}

func (s *AlertStore) AcknowledgeAlert(id string, userID uint) error {
	now := time.Now()
	return s.db.Model(&models.Alert{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":          "acknowledged",
		"acknowledged_at": now,
		"acknowledged_by": userID,
	}).Error
}

func (s *AlertStore) ResolveAlert(id string) error {
	now := time.Now()
	return s.db.Model(&models.Alert{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":      "resolved",
		"resolved_at": now,
	}).Error
}

func (s *AlertStore) GetActiveAlertCount(userID uint) (int64, error) {
	var count int64
	err := s.db.Model(&models.Alert{}).Where("user_id = ? AND status = ?", userID, "active").Count(&count).Error
	return count, err
}

func (s *AlertStore) GetAlertStats(userID uint) (*models.AlertStats, error) {
	stats := &models.AlertStats{}

	err := s.db.Model(&models.Alert{}).Where("user_id = ? AND status = ?", userID, "active").Count(&stats.ActiveCount).Error
	if err != nil {
		return nil, err
	}

	err = s.db.Model(&models.Alert{}).Where("user_id = ? AND status = ?", userID, "acknowledged").Count(&stats.AcknowledgedCount).Error
	if err != nil {
		return nil, err
	}

	today := time.Now().Format("2006-01-02")
	err = s.db.Model(&models.Alert{}).Where("user_id = ? AND DATE(created_at) = ?", userID, today).Count(&stats.TodayCount).Error
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func (s *AlertStore) CleanupOldAlerts(days int) error {
	before := time.Now().AddDate(0, 0, -days)
	return s.db.Where("created_at < ? AND status != ?", before, "active").Delete(&models.Alert{}).Error
}

func (s *AlertStore) GetDeviceOwner(deviceID string) (uint, error) {
	var device models.Device
	err := s.db.First(&device, "id = ?", deviceID).Error
	if err != nil {
		return 0, err
	}
	return device.UserID, nil
}

func (s *AlertStore) GetDevice(deviceID string) (*models.Device, error) {
	var device models.Device
	err := s.db.First(&device, "id = ?", deviceID).Error
	if err != nil {
		return nil, err
	}
	return &device, nil
}
