package firmware

import (
	"fmt"
	"time"

	"iot-platform/pkg/models"

	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CreateFirmware(fw *models.Firmware) error {
	return s.db.Create(fw).Error
}

func (s *Store) GetFirmware(id uint) (*models.Firmware, error) {
	var fw models.Firmware
	if err := s.db.First(&fw, id).Error; err != nil {
		return nil, err
	}
	return &fw, nil
}

func (s *Store) ListFirmwares(deviceType string) ([]*models.Firmware, error) {
	var firmwares []*models.Firmware
	query := s.db.Model(&models.Firmware{})
	if deviceType != "" {
		query = query.Where("device_type = ?", deviceType)
	}
	if err := query.Order("created_at DESC").Find(&firmwares).Error; err != nil {
		return nil, err
	}
	return firmwares, nil
}

func (s *Store) DeleteFirmware(id uint) error {
	return s.db.Delete(&models.Firmware{}, id).Error
}

func (s *Store) GetDeviceFirmware(deviceID string) (*models.DeviceFirmware, error) {
	var df models.DeviceFirmware
	if err := s.db.Where("device_id = ?", deviceID).First(&df).Error; err != nil {
		return nil, err
	}
	return &df, nil
}

func (s *Store) SetDeviceFirmware(deviceID string, firmwareID uint, version string) error {
	df := &models.DeviceFirmware{
		DeviceID:   deviceID,
		FirmwareID: firmwareID,
		Version:    version,
		UpgradedAt: time.Now(),
	}
	return s.db.Save(df).Error
}

func (s *Store) CreateUpgradeTask(task *models.UpgradeTask) error {
	return s.db.Create(task).Error
}

func (s *Store) GetUpgradeTask(id string) (*models.UpgradeTask, error) {
	var task models.UpgradeTask
	if err := s.db.Where("id = ?", id).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *Store) ListUpgradeTasks(limit, offset int) ([]*models.UpgradeTask, int64, error) {
	var tasks []*models.UpgradeTask
	var total int64

	s.db.Model(&models.UpgradeTask{}).Count(&total)
	if err := s.db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func (s *Store) UpdateUpgradeTask(task *models.UpgradeTask) error {
	return s.db.Save(task).Error
}

func (s *Store) CreateUpgradeTaskDevice(taskDevice *models.UpgradeTaskDevice) error {
	return s.db.Create(taskDevice).Error
}

func (s *Store) GetUpgradeTaskDevice(id string) (*models.UpgradeTaskDevice, error) {
	var td models.UpgradeTaskDevice
	if err := s.db.Where("id = ?", id).First(&td).Error; err != nil {
		return nil, err
	}
	return &td, nil
}

func (s *Store) GetUpgradeTaskDevices(taskID string) ([]*models.UpgradeTaskDevice, error) {
	var devices []*models.UpgradeTaskDevice
	if err := s.db.Where("task_id = ?", taskID).Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

func (s *Store) UpdateUpgradeTaskDevice(td *models.UpgradeTaskDevice) error {
	return s.db.Save(td).Error
}

func (s *Store) GetPendingUpgradeTaskDevices(deviceID string) ([]*models.UpgradeTaskDevice, error) {
	var devices []*models.UpgradeTaskDevice
	if err := s.db.Where("device_id = ? AND status IN ('pending', 'downloading', 'installing')", deviceID).
		Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

func (s *Store) GetDeviceIDsByType(deviceType string) ([]string, error) {
	var devices []*models.Device
	query := s.db

	if deviceType != "" {
		var dt models.DeviceType
		if err := s.db.Where("name = ?", deviceType).First(&dt).Error; err != nil {
			return nil, fmt.Errorf("device type not found: %w", err)
		}
		query = query.Where("type_id = ?", dt.ID)
	}

	if err := query.Find(&devices).Error; err != nil {
		return nil, err
	}

	ids := make([]string, len(devices))
	for i, d := range devices {
		ids[i] = d.ID
	}
	return ids, nil
}
