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
		&models.User{},
		&models.RefreshToken{},
		&models.Device{},
		&models.TelemetryData{},
		&models.DeviceCommand{},
		&models.DeviceType{},
		&models.Firmware{},
		&models.DeviceFirmware{},
		&models.UpgradeTask{},
		&models.UpgradeTaskDevice{},
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

func (s *Store) GetDeviceCountByUserID(userID uint) (int64, error) {
	var count int64
	err := s.db.Model(&models.Device{}).Where("user_id = ?", userID).Count(&count).Error
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

func (s *Store) GetDeviceFirmwareVersion(deviceID string) string {
	var df models.DeviceFirmware
	if err := s.db.Where("device_id = ?", deviceID).First(&df).Error; err != nil {
		return ""
	}
	return df.Version
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

	userNames := make(map[uint]string)
	var users []*models.User
	s.db.Find(&users)
	for _, u := range users {
		userNames[u.ID] = u.Username
	}

	result := make([]*models.DeviceResponse, 0, len(devices))
	for _, d := range devices {
		resp := &models.DeviceResponse{
			ID:              d.ID,
			Name:            d.Name,
			TypeID:          d.TypeID,
			UserID:          d.UserID,
			Status:          string(d.Status),
			Secret:          d.Secret,
			Disabled:        d.Disabled,
			Properties:      d.Properties,
			LastSeen:        d.LastSeen.Format(time.RFC3339),
			CreatedAt:       d.CreatedAt.Format(time.RFC3339),
			FirmwareVersion: s.GetDeviceFirmwareVersion(d.ID),
		}
		if name, ok := typeNames[d.TypeID]; ok {
			resp.TypeName = name
		}
		if ownerName, ok := userNames[d.UserID]; ok {
			resp.OwnerName = ownerName
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

	ownerName := ""
	if d.UserID > 0 {
		if u, err := s.GetUserByID(d.UserID); err == nil {
			ownerName = u.Username
		}
	}

	return &models.DeviceResponse{
		ID:         d.ID,
		Name:       d.Name,
		TypeID:     d.TypeID,
		TypeName:   typeName,
		UserID:     d.UserID,
		OwnerName:  ownerName,
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

func (s *Store) CreateUser(user *models.User) error {
	return s.db.Create(user).Error
}

func (s *Store) GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	err := s.db.First(&user, "username = ?", username).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	err := s.db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) GetAllUsers() ([]*models.User, error) {
	var users []*models.User
	err := s.db.Order("id ASC").Find(&users).Error
	return users, err
}

func (s *Store) UpdateUser(id uint, updates map[string]interface{}) error {
	return s.db.Model(&models.User{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Store) DeleteUser(id uint) error {
	return s.db.Delete(&models.User{}, "id = ?", id).Error
}

func (s *Store) DisableUser(id uint) error {
	return s.db.Model(&models.User{}).Where("id = ?", id).Update("disabled", true).Error
}

func (s *Store) EnableUser(id uint) error {
	return s.db.Model(&models.User{}).Where("id = ?", id).Update("disabled", false).Error
}

func (s *Store) SaveRefreshToken(token *models.RefreshToken) error {
	return s.db.Create(token).Error
}

func (s *Store) GetRefreshToken(token string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	err := s.db.First(&rt, "token = ?", token).Error
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func (s *Store) DeleteRefreshToken(token string) error {
	return s.db.Delete(&models.RefreshToken{}, "token = ?", token).Error
}

func (s *Store) DeleteUserRefreshTokens(userID uint) error {
	return s.db.Delete(&models.RefreshToken{}, "user_id = ?", userID).Error
}

func (s *Store) CleanupExpiredRefreshTokens() error {
	return s.db.Where("expires_at < ?", time.Now()).Delete(&models.RefreshToken{}).Error
}

func (s *Store) GetDeviceByID(id string) (*models.Device, error) {
	var device models.Device
	err := s.db.First(&device, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &device, nil
}

func (s *Store) ListDevicesByUserID(userID uint) ([]*models.DeviceResponse, error) {
	var devices []*models.Device
	if err := s.db.Where("user_id = ?", userID).Find(&devices).Error; err != nil {
		return nil, err
	}

	typeNames := make(map[uint]string)
	var types []*models.DeviceType
	s.db.Find(&types)
	for _, t := range types {
		typeNames[t.ID] = t.Name
	}

	ownerName := ""
	if u, err := s.GetUserByID(userID); err == nil {
		ownerName = u.Username
	}

	result := make([]*models.DeviceResponse, 0, len(devices))
	for _, d := range devices {
		resp := &models.DeviceResponse{
			ID:         d.ID,
			Name:       d.Name,
			TypeID:     d.TypeID,
			UserID:     d.UserID,
			OwnerName:  ownerName,
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

func (s *Store) GetDeviceWithTypeAndUser(deviceID string, userID uint, role string) (*models.DeviceResponse, error) {
	var d models.Device
	err := s.db.First(&d, "id = ?", deviceID).Error
	if err != nil {
		return nil, err
	}

	if role != "admin" && d.UserID != userID {
		return nil, fmt.Errorf("access denied")
	}

	typeName := ""
	if d.TypeID > 0 {
		if dt, err := s.GetDeviceTypeName(d.TypeID); err == nil {
			typeName = dt
		}
	}

	ownerName := ""
	if d.UserID > 0 {
		if u, err := s.GetUserByID(d.UserID); err == nil {
			ownerName = u.Username
		}
	}

	return &models.DeviceResponse{
		ID:              d.ID,
		Name:            d.Name,
		TypeID:          d.TypeID,
		TypeName:        typeName,
		UserID:          d.UserID,
		OwnerName:       ownerName,
		Status:          string(d.Status),
		Secret:          d.Secret,
		Disabled:        d.Disabled,
		Properties:      d.Properties,
		LastSeen:        d.LastSeen.Format(time.RFC3339),
		CreatedAt:       d.CreatedAt.Format(time.RFC3339),
		FirmwareVersion: s.GetDeviceFirmwareVersion(d.ID),
	}, nil
}

func (s *Store) ListDevicesWithUserID() ([]*models.Device, error) {
	var devices []*models.Device
	err := s.db.Find(&devices).Error
	return devices, err
}
