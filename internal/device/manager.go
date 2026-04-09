package device

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"iot-platform/internal/storage"
	"iot-platform/pkg/models"

	"gorm.io/gorm"
)

type Manager struct {
	db       *gorm.DB
	storage  *storage.Store
	devices  map[string]*DeviceWrapper
	lock     sync.RWMutex
	onUpdate func(*models.Device)
}

type DeviceWrapper struct {
	*models.Device
	heartbeatTimer *time.Timer
}

func NewManager(db *gorm.DB, store *storage.Store) *Manager {
	return &Manager{
		db:      db,
		storage: store,
		devices: make(map[string]*DeviceWrapper),
	}
}

func (m *Manager) SetUpdateCallback(cb func(*models.Device)) {
	m.onUpdate = cb
}

func (m *Manager) ensureDeviceType(deviceType string) (uint, string) {
	if deviceType == "" {
		return 0, ""
	}

	if dt, err := m.storage.GetDeviceTypeByName(deviceType); err == nil && dt != nil {
		return dt.ID, dt.Name
	}

	dt, err := m.storage.CreateDeviceType(deviceType)
	if err == nil && dt != nil {
		return dt.ID, dt.Name
	}

	if dt, err = m.storage.GetDeviceTypeByName(deviceType); err == nil && dt != nil {
		return dt.ID, dt.Name
	}

	return 0, deviceType
}

func (m *Manager) Register(name, deviceType string, props models.Properties, userID uint) (*models.Device, error) {
	typeID, _ := m.ensureDeviceType(deviceType)

	device := &models.Device{
		ID:        uuid.New().String(),
		Name:      name,
		TypeID:    typeID,
		UserID:    userID,
		Status:    models.StatusOffline,
		Secret:    uuid.New().String(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	device.SetProperties(props)

	if err := m.db.Create(device).Error; err != nil {
		return nil, err
	}

	m.lock.Lock()
	m.devices[device.ID] = &DeviceWrapper{Device: device}
	m.lock.Unlock()

	return device, nil
}

func (m *Manager) Unregister(deviceID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if _, ok := m.devices[deviceID]; ok {
		delete(m.devices, deviceID)
		return m.db.Delete(&models.Device{}, "id = ?", deviceID).Error
	}
	return fmt.Errorf("device not found")
}

func (m *Manager) GetDevice(deviceID string) (*models.Device, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if d, ok := m.devices[deviceID]; ok {
		return d.Device, nil
	}

	var device models.Device
	if err := m.db.First(&device, "id = ?", deviceID).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (m *Manager) ListDevices() ([]*models.Device, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	var devices []*models.Device
	if err := m.db.Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

func (m *Manager) UpdateStatus(deviceID string, status models.DeviceStatus) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	device, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found")
	}

	device.Status = status
	device.LastSeen = time.Now()

	updates := map[string]interface{}{
		"status":    status,
		"last_seen": device.LastSeen,
	}

	if err := m.db.Model(&models.Device{}).Where("id = ?", deviceID).Updates(updates).Error; err != nil {
		return err
	}

	if m.onUpdate != nil {
		go m.onUpdate(device.Device)
	}

	return nil
}

func (m *Manager) UpdateDeviceInfo(deviceID, name, deviceType string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	device, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found")
	}

	updates := map[string]interface{}{}
	if name != "" {
		device.Name = name
		updates["name"] = name
	}
	if deviceType != "" {
		typeID, _ := m.ensureDeviceType(deviceType)
		device.TypeID = typeID
		updates["type_id"] = typeID
	}

	if len(updates) > 0 {
		if err := m.db.Model(&models.Device{}).Where("id = ?", deviceID).Updates(updates).Error; err != nil {
			return err
		}
	}

	if m.onUpdate != nil {
		go m.onUpdate(device.Device)
	}

	return nil
}

func (m *Manager) UpdateDeviceOwner(deviceID string, userID uint) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	device, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found")
	}

	device.UserID = userID

	if err := m.db.Model(&models.Device{}).Where("id = ?", deviceID).Update("user_id", userID).Error; err != nil {
		return err
	}

	if m.onUpdate != nil {
		go m.onUpdate(device.Device)
	}

	return nil
}

func (m *Manager) UpdateProperties(deviceID string, props models.Properties) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	device, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found")
	}

	device.SetProperties(props)
	device.LastSeen = time.Now()

	updates := map[string]interface{}{
		"properties": device.Properties,
		"last_seen":  device.LastSeen,
	}

	if err := m.db.Model(&models.Device{}).Where("id = ?", deviceID).Updates(updates).Error; err != nil {
		return err
	}

	if m.onUpdate != nil {
		go m.onUpdate(device.Device)
	}

	return nil
}

func (m *Manager) HandleHeartbeat(deviceID string) error {
	return m.UpdateStatus(deviceID, models.StatusOnline)
}

func (m *Manager) LoadFromDB() error {
	var devices []models.Device
	if err := m.db.Find(&devices).Error; err != nil {
		return err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	for i := range devices {
		m.devices[devices[i].ID] = &DeviceWrapper{Device: &devices[i]}
	}

	return nil
}

func (m *Manager) GetStats() map[string]interface{} {
	m.lock.RLock()
	defer m.lock.RUnlock()

	stats := map[string]interface{}{
		"total":   len(m.devices),
		"online":  0,
		"offline": 0,
	}

	for _, d := range m.devices {
		if d.Status == models.StatusOnline {
			stats["online"] = stats["online"].(int) + 1
		} else {
			stats["offline"] = stats["offline"].(int) + 1
		}
	}

	return stats
}

func (m *Manager) DisableDevice(deviceID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	device, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found")
	}

	device.Disabled = true
	device.Status = models.StatusOffline

	updates := map[string]interface{}{
		"disabled": true,
		"status":   models.StatusOffline,
	}

	if err := m.db.Model(&models.Device{}).Where("id = ?", deviceID).Updates(updates).Error; err != nil {
		return err
	}

	if m.onUpdate != nil {
		go m.onUpdate(device.Device)
	}

	return nil
}

func (m *Manager) EnableDevice(deviceID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	device, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found")
	}

	device.Disabled = false

	updates := map[string]interface{}{
		"disabled": false,
	}

	if err := m.db.Model(&models.Device{}).Where("id = ?", deviceID).Updates(updates).Error; err != nil {
		return err
	}

	if m.onUpdate != nil {
		go m.onUpdate(device.Device)
	}

	return nil
}
