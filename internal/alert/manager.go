package alert

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"iot-platform/pkg/models"

	"github.com/google/uuid"
)

type Manager struct {
	store     *AlertStore
	evaluator *Evaluator
	executor  *Executor
	rules     map[string]*models.AlertRule
	lock      sync.RWMutex
}

func NewManager(store *AlertStore, evaluator *Evaluator, executor *Executor) *Manager {
	return &Manager{
		store:     store,
		evaluator: evaluator,
		executor:  executor,
		rules:     make(map[string]*models.AlertRule),
	}
}

func (m *Manager) Initialize() error {
	if err := m.store.AutoMigrate(); err != nil {
		return fmt.Errorf("failed to migrate alert tables: %w", err)
	}

	if err := m.LoadRules(); err != nil {
		return fmt.Errorf("failed to load alert rules: %w", err)
	}

	go m.cleanupLoop()

	return nil
}

func (m *Manager) LoadRules() error {
	rules, err := m.store.ListAllEnabledRules()
	if err != nil {
		return err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	for _, rule := range rules {
		m.rules[rule.ID] = rule
	}

	log.Printf("[Alert] Loaded %d enabled rules", len(rules))
	return nil
}

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		if err := m.store.CleanupOldAlerts(7); err != nil {
			log.Printf("[Alert] Failed to cleanup old alerts: %v", err)
		}
	}
}

func (m *Manager) CreateRule(userID uint, req *models.AlertRuleRequest) (*models.AlertRule, error) {
	conditionsJSON, err := json.Marshal(req.Conditions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal conditions: %w", err)
	}

	actionsJSON, err := json.Marshal(req.Actions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal actions: %w", err)
	}

	priority := req.Priority
	if priority == 0 {
		priority = models.PriorityMedium
	}

	checkInterval := req.CheckInterval
	if checkInterval == 0 {
		checkInterval = 60
	}

	cooldown := req.Cooldown
	if cooldown == 0 {
		cooldown = 300
	}

	rule := &models.AlertRule{
		ID:            uuid.New().String(),
		Name:          req.Name,
		Description:   req.Description,
		UserID:        userID,
		DeviceID:      req.DeviceID,
		DeviceType:    req.DeviceType,
		ConditionType: req.ConditionType,
		Conditions:    string(conditionsJSON),
		Expression:    req.Expression,
		Actions:       string(actionsJSON),
		Priority:      priority,
		Enabled:       req.Enabled,
		CheckInterval: checkInterval,
		Cooldown:      cooldown,
	}

	if err := m.store.CreateRule(rule); err != nil {
		return nil, err
	}

	m.lock.Lock()
	m.rules[rule.ID] = rule
	m.lock.Unlock()

	return rule, nil
}

func (m *Manager) UpdateRule(id string, userID uint, req *models.AlertRuleRequest) (*models.AlertRule, error) {
	rule, err := m.store.GetRule(id)
	if err != nil {
		return nil, err
	}

	if rule.UserID != userID {
		return nil, fmt.Errorf("access denied")
	}

	if req.Name != "" {
		rule.Name = req.Name
	}
	if req.Description != "" {
		rule.Description = req.Description
	}
	if req.DeviceID != "" {
		rule.DeviceID = req.DeviceID
	}
	if req.DeviceType != "" {
		rule.DeviceType = req.DeviceType
	}
	if req.ConditionType != "" {
		rule.ConditionType = req.ConditionType
	}
	if len(req.Conditions) > 0 {
		conditionsJSON, _ := json.Marshal(req.Conditions)
		rule.Conditions = string(conditionsJSON)
	}
	if req.Expression != "" {
		rule.Expression = req.Expression
	}
	if len(req.Actions) > 0 {
		actionsJSON, _ := json.Marshal(req.Actions)
		rule.Actions = string(actionsJSON)
	}
	if req.Priority != 0 {
		rule.Priority = req.Priority
	}
	if req.CheckInterval > 0 {
		rule.CheckInterval = req.CheckInterval
	}
	if req.Cooldown > 0 {
		rule.Cooldown = req.Cooldown
	}
	rule.Enabled = req.Enabled

	if err := m.store.UpdateRule(rule); err != nil {
		return nil, err
	}

	m.lock.Lock()
	if rule.Enabled {
		m.rules[rule.ID] = rule
	} else {
		delete(m.rules, rule.ID)
	}
	m.lock.Unlock()

	return rule, nil
}

func (m *Manager) DeleteRule(id string, userID uint) error {
	rule, err := m.store.GetRule(id)
	if err != nil {
		return err
	}

	if rule.UserID != userID {
		return fmt.Errorf("access denied")
	}

	if err := m.store.DeleteRule(id); err != nil {
		return err
	}

	m.lock.Lock()
	delete(m.rules, id)
	m.lock.Unlock()

	return nil
}

func (m *Manager) GetRule(id string, userID uint) (*models.AlertRule, error) {
	rule, err := m.store.GetRule(id)
	if err != nil {
		return nil, err
	}

	if rule.UserID != userID {
		return nil, fmt.Errorf("access denied")
	}

	return rule, nil
}

func (m *Manager) ListRules(userID uint) ([]*models.AlertRule, error) {
	return m.store.ListRules(userID)
}

func (m *Manager) EnableRule(id string, userID uint) error {
	rule, err := m.store.GetRule(id)
	if err != nil {
		return err
	}

	if rule.UserID != userID {
		return fmt.Errorf("access denied")
	}

	rule.Enabled = true
	if err := m.store.UpdateRule(rule); err != nil {
		return err
	}

	m.lock.Lock()
	m.rules[rule.ID] = rule
	m.lock.Unlock()

	return nil
}

func (m *Manager) DisableRule(id string, userID uint) error {
	rule, err := m.store.GetRule(id)
	if err != nil {
		return err
	}

	if rule.UserID != userID {
		return fmt.Errorf("access denied")
	}

	rule.Enabled = false
	if err := m.store.UpdateRule(rule); err != nil {
		return err
	}

	m.lock.Lock()
	delete(m.rules, id)
	m.lock.Unlock()

	return nil
}

func (m *Manager) GetRulesForDevice(deviceID string) []*models.AlertRule {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var matched []*models.AlertRule
	for _, rule := range m.rules {
		if rule.DeviceID == "" || rule.DeviceID == deviceID {
			matched = append(matched, rule)
		}
	}

	return matched
}

func (m *Manager) ProcessTelemetry(deviceID string, data map[string]interface{}) {
	rules := m.GetRulesForDevice(deviceID)
	if len(rules) == 0 {
		return
	}

	device, err := m.store.GetDevice(deviceID)
	if err != nil {
		log.Printf("[Alert] Failed to get device %s: %v", deviceID, err)
		return
	}

	for _, rule := range rules {
		if !m.checkCooldown(rule) {
			continue
		}

		triggered, value, err := m.evaluator.Evaluate(rule, device, data)
		if err != nil {
			log.Printf("[Alert] Rule %s evaluation error: %v", rule.ID, err)
			continue
		}

		if triggered {
			go m.triggerAlert(rule, device, value)
		}
	}
}

func (m *Manager) checkCooldown(rule *models.AlertRule) bool {
	if rule.LastTriggered == nil {
		return true
	}

	cooldownDuration := time.Duration(rule.Cooldown) * time.Second
	return time.Since(*rule.LastTriggered) >= cooldownDuration
}

func (m *Manager) triggerAlert(rule *models.AlertRule, device *models.Device, value interface{}) {
	now := time.Now()
	rule.LastTriggered = &now
	m.store.UpdateLastTriggered(rule.ID, now)

	triggerValue := fmt.Sprintf("%v", value)
	message := fmt.Sprintf("%s: %s", rule.Name, triggerValue)

	alert := &models.Alert{
		ID:           uuid.New().String(),
		RuleID:       rule.ID,
		RuleName:     rule.Name,
		DeviceID:     device.ID,
		DeviceName:   device.Name,
		UserID:       device.UserID,
		Status:       "active",
		Priority:     rule.Priority,
		TriggerValue: triggerValue,
		Message:      message,
		CreatedAt:    now,
	}

	if err := m.store.CreateAlert(alert); err != nil {
		log.Printf("[Alert] Failed to create alert: %v", err)
		return
	}

	if err := m.executor.Execute(alert, rule, device); err != nil {
		log.Printf("[Alert] Failed to execute alert: %v", err)
	}
}

func (m *Manager) GetAlerts(userID uint, status string, limit, offset int) ([]*models.Alert, int64, error) {
	return m.store.ListAlerts(userID, status, limit, offset)
}

func (m *Manager) AcknowledgeAlert(alertID string, userID uint) error {
	return m.store.AcknowledgeAlert(alertID, userID)
}

func (m *Manager) ResolveAlert(alertID string, userID uint) error {
	return m.store.ResolveAlert(alertID)
}

func (m *Manager) GetAlertStats(userID uint) (*models.AlertStats, error) {
	return m.store.GetAlertStats(userID)
}
