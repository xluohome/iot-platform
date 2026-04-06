package storage

import (
	"fmt"
	"time"

	"iot-platform/internal/config"
	"iot-platform/pkg/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db *gorm.DB
}

func New(cfg *config.DatabaseConfig) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(cfg.Path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	if err := db.AutoMigrate(
		&models.Device{},
		&models.TelemetryData{},
		&models.DeviceCommand{},
		&models.DeviceType{},
	); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	s := &Store{db: db}
	s.initPresetTypes()

	return s, nil
}

func (s *Store) DB() *gorm.DB {
	return s.db
}

func (s *Store) SaveTelemetry(deviceID, data string, timestamp time.Time) error {
	record := &models.TelemetryData{
		DeviceID:  deviceID,
		Data:      data,
		Timestamp: timestamp,
	}
	return s.db.Create(record).Error
}

func (s *Store) GetTelemetry(deviceID string, limit int) ([]*models.TelemetryData, error) {
	var records []*models.TelemetryData
	err := s.db.Where("device_id = ?", deviceID).
		Order("timestamp DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

func (s *Store) GetLatestTelemetry(deviceID string) (*models.TelemetryData, error) {
	var record models.TelemetryData
	err := s.db.Where("device_id = ?", deviceID).
		Order("timestamp DESC").
		First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Store) SaveCommand(deviceID, command, params string) (*models.DeviceCommand, error) {
	record := &models.DeviceCommand{
		DeviceID: deviceID,
		Command:  command,
		Params:   params,
		Status:   "pending",
	}
	if err := s.db.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Store) UpdateCommandStatus(commandID, status, result string) error {
	updates := map[string]interface{}{
		"status": status,
		"result": result,
	}
	return s.db.Model(&models.DeviceCommand{}).
		Where("id = ?", commandID).
		Updates(updates).Error
}

func (s *Store) GetCommands(deviceID string, limit int) ([]*models.DeviceCommand, error) {
	var commands []*models.DeviceCommand
	err := s.db.Where("device_id = ?", deviceID).
		Order("created_at DESC").
		Limit(limit).
		Find(&commands).Error
	return commands, err
}

func (s *Store) GetCommand(commandID string) (*models.DeviceCommand, error) {
	var command models.DeviceCommand
	err := s.db.First(&command, "id = ?", commandID).Error
	if err != nil {
		return nil, err
	}
	return &command, nil
}

func (s *Store) initPresetTypes() {
	presets := []string{"sensor", "actuator", "gateway", "camera", "thermostat", "light", "switch", "meter"}
	for _, name := range presets {
		var existing models.DeviceType
		s.db.FirstOrCreate(&existing, models.DeviceType{Name: name})
	}
}

func (s *Store) GetAllDeviceTypes() ([]*models.DeviceType, error) {
	var types []*models.DeviceType
	err := s.db.Order("id ASC").Find(&types).Error
	return types, err
}

func (s *Store) CreateDeviceType(name string) (*models.DeviceType, error) {
	dt := &models.DeviceType{Name: name}
	err := s.db.Create(dt).Error
	return dt, err
}

func (s *Store) UpdateDeviceType(id uint, name string) error {
	return s.db.Model(&models.DeviceType{}).Where("id = ?", id).Update("name", name).Error
}

func (s *Store) DeleteDeviceType(id uint) error {
	return s.db.Delete(&models.DeviceType{}, "id = ?", id).Error
}

func (s *Store) GetDeviceCountByType(typeID uint) (int64, error) {
	var count int64
	err := s.db.Model(&models.Device{}).Where("type_id = ?", typeID).Count(&count).Error
	return count, err
}

func (s *Store) GetDeviceTypeName(typeID uint) (string, error) {
	var dt models.DeviceType
	err := s.db.First(&dt, "id = ?", typeID).Error
	if err != nil {
		return "", err
	}
	return dt.Name, nil
}

func (s *Store) GetDeviceTypeByName(name string) (*models.DeviceType, error) {
	var dt models.DeviceType
	err := s.db.First(&dt, "name = ?", name).Error
	if err != nil {
		return nil, err
	}
	return &dt, nil
}

func (s *Store) ListDevicesWithTypes() ([]*models.DeviceResponse, error) {
	var devices []*models.Device
	if err := s.db.Find(&devices).Error; err != nil {
		return nil, err
	}

	typeNames := make(map[uint]string)
	var types []*models.DeviceType
	s.db.Find(&types)
	for _, t := range types {
		typeNames[t.ID] = t.Name
	}

	result := make([]*models.DeviceResponse, 0, len(devices))
	for _, d := range devices {
		resp := &models.DeviceResponse{
			ID:         d.ID,
			Name:       d.Name,
			TypeID:     d.TypeID,
			Status:     string(d.Status),
			Secret:     d.Secret,
			Disabled:   d.Disabled,
			Properties: d.Properties,
			LastSeen:   d.LastSeen.Format(time.RFC3339),
			CreatedAt:  d.CreatedAt.Format(time.RFC3339),
		}
		if name, ok := typeNames[d.TypeID]; ok {
			resp.TypeName = name
		}
		result = append(result, resp)
	}
	return result, nil
}

func (s *Store) GetDeviceWithType(deviceID string) (*models.DeviceResponse, error) {
	var d models.Device
	if err := s.db.First(&d, "id = ?", deviceID).Error; err != nil {
		return nil, err
	}

	typeName := ""
	if d.TypeID > 0 {
		if dt, err := s.GetDeviceTypeName(d.TypeID); err == nil {
			typeName = dt
		}
	}

	return &models.DeviceResponse{
		ID:         d.ID,
		Name:       d.Name,
		TypeID:     d.TypeID,
		TypeName:   typeName,
		Status:     string(d.Status),
		Secret:     d.Secret,
		Disabled:   d.Disabled,
		Properties: d.Properties,
		LastSeen:   d.LastSeen.Format(time.RFC3339),
		CreatedAt:  d.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *Store) CleanupOldTelemetry(days int) error {
	before := time.Now().AddDate(0, 0, -days)
	return s.db.Where("timestamp < ?", before).Delete(&models.TelemetryData{}).Error
}
