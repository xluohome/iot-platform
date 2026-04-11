package firmware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"iot-platform/internal/mqtt"
	"iot-platform/pkg/models"

	"github.com/google/uuid"
)

const (
	MaxFirmwareSize = 16 * 1024 * 1024 // 16MB
	FirmwareDir     = "./firmware"
)

type Manager struct {
	store      *Store
	mqttServer *mqtt.Server
}

func NewManager(store *Store, mqttServer *mqtt.Server) *Manager {
	return &Manager{
		store:      store,
		mqttServer: mqttServer,
	}
}

func (m *Manager) UploadFirmware(name, version, deviceType, description string, file io.Reader, fileSize int64) (*models.Firmware, error) {
	if fileSize > MaxFirmwareSize {
		return nil, fmt.Errorf("firmware size exceeds maximum limit of %d bytes", MaxFirmwareSize)
	}

	fwID := uuid.New().String()
	filePath := filepath.Join(FirmwareDir, fwID+".bin")

	if err := os.MkdirAll(FirmwareDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create firmware directory: %w", err)
	}

	dst, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create firmware file: %w", err)
	}
	defer dst.Close()

	hash := sha256.New()
	writer := io.MultiWriter(dst, hash)

	if _, err := io.Copy(writer, file); err != nil {
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to save firmware: %w", err)
	}

	fw := &models.Firmware{
		Name:        name,
		Version:     version,
		DeviceType:  deviceType,
		FilePath:    filePath,
		FileSize:    fileSize,
		Checksum:    hex.EncodeToString(hash.Sum(nil)),
		Description: description,
		CreatedAt:   time.Now(),
	}

	if err := m.store.CreateFirmware(fw); err != nil {
		os.Remove(filePath)
		return nil, err
	}

	return fw, nil
}

func (m *Manager) GetFirmware(id uint) (*models.Firmware, error) {
	return m.store.GetFirmware(id)
}

func (m *Manager) ListFirmwares(deviceType string) ([]*models.Firmware, error) {
	return m.store.ListFirmwares(deviceType)
}

func (m *Manager) DeleteFirmware(id uint) error {
	fw, err := m.store.GetFirmware(id)
	if err != nil {
		return err
	}

	os.Remove(fw.FilePath)
	return m.store.DeleteFirmware(id)
}

func (m *Manager) GetDeviceFirmware(deviceID string) (*models.DeviceFirmware, error) {
	return m.store.GetDeviceFirmware(deviceID)
}

func (m *Manager) UpgradeDevice(deviceID string, firmwareID uint) (*models.UpgradeTaskDevice, error) {
	fw, err := m.store.GetFirmware(firmwareID)
	if err != nil {
		return nil, fmt.Errorf("firmware not found: %w", err)
	}

	df, _ := m.store.GetDeviceFirmware(deviceID)
	oldVersion := ""
	if df != nil {
		oldVersion = df.Version
	}

	taskID := uuid.New().String()
	taskDeviceID := uuid.New().String()

	taskDevice := &models.UpgradeTaskDevice{
		ID:         taskDeviceID,
		TaskID:     taskID,
		DeviceID:   deviceID,
		OldVersion: oldVersion,
		NewVersion: fw.Version,
		Status:     "pending",
		Progress:   0,
	}

	task := &models.UpgradeTask{
		ID:              taskID,
		FirmwareID:      firmwareID,
		FirmwareName:    fw.Name,
		FirmwareVersion: fw.Version,
		TargetType:      fw.DeviceType,
		TotalDevices:    1,
		SelectedDevices: 1,
		Percentage:      100,
		Status:          "in_progress",
		CreatedAt:       time.Now(),
	}

	if err := m.store.CreateUpgradeTask(task); err != nil {
		return nil, err
	}

	if err := m.store.CreateUpgradeTaskDevice(taskDevice); err != nil {
		return nil, err
	}

	m.notifyDeviceFirmware(deviceID, taskDeviceID, fw)

	return taskDevice, nil
}

func (m *Manager) CreateUpgradeTaskByPercentage(firmwareID uint, percentage int) (*models.UpgradeTask, error) {
	fw, err := m.store.GetFirmware(firmwareID)
	if err != nil {
		return nil, fmt.Errorf("firmware not found: %w", err)
	}

	deviceIDs, err := m.store.GetDeviceIDsByType(fw.DeviceType)
	if err != nil {
		return nil, fmt.Errorf("failed to get devices: %w", err)
	}

	if len(deviceIDs) == 0 {
		return nil, fmt.Errorf("no devices found for type: %s", fw.DeviceType)
	}

	totalDevices := len(deviceIDs)
	selectedCount := totalDevices
	if percentage < 100 {
		selectedCount = (totalDevices * percentage) / 100
		if selectedCount < 1 {
			selectedCount = 1
		}
	}

	selectedDeviceIDs := m.selectRandomDevices(deviceIDs, selectedCount)

	taskID := uuid.New().String()
	task := &models.UpgradeTask{
		ID:              taskID,
		FirmwareID:      firmwareID,
		FirmwareName:    fw.Name,
		FirmwareVersion: fw.Version,
		TargetType:      fw.DeviceType,
		TotalDevices:    totalDevices,
		SelectedDevices: selectedCount,
		Percentage:      percentage,
		Status:          "in_progress",
		CreatedAt:       time.Now(),
	}

	if err := m.store.CreateUpgradeTask(task); err != nil {
		return nil, err
	}

	for _, deviceID := range selectedDeviceIDs {
		df, _ := m.store.GetDeviceFirmware(deviceID)
		oldVersion := ""
		if df != nil {
			oldVersion = df.Version
		}

		taskDeviceID := uuid.New().String()
		taskDevice := &models.UpgradeTaskDevice{
			ID:         taskDeviceID,
			TaskID:     taskID,
			DeviceID:   deviceID,
			OldVersion: oldVersion,
			NewVersion: fw.Version,
			Status:     "pending",
			Progress:   0,
		}

		if err := m.store.CreateUpgradeTaskDevice(taskDevice); err != nil {
			log.Printf("Failed to create task device: %v", err)
			continue
		}

		m.notifyDeviceFirmware(deviceID, taskDeviceID, fw)
	}

	return task, nil
}

func (m *Manager) selectRandomDevices(deviceIDs []string, count int) []string {
	if count >= len(deviceIDs) {
		return deviceIDs
	}

	shuffled := make([]string, len(deviceIDs))
	copy(shuffled, deviceIDs)

	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled[:count]
}

func (m *Manager) ExpandTask(taskID string, newPercentage int) error {
	task, err := m.store.GetUpgradeTask(taskID)
	if err != nil {
		return err
	}

	if task.Status != "in_progress" {
		return fmt.Errorf("task is not in progress")
	}

	if newPercentage <= task.Percentage {
		return fmt.Errorf("new percentage must be greater than current percentage")
	}

	deviceIDs, err := m.store.GetDeviceIDsByType(task.TargetType)
	if err != nil {
		return err
	}

	existingDevices, err := m.store.GetUpgradeTaskDevices(taskID)
	if err != nil {
		return err
	}

	existingDeviceIDs := make(map[string]bool)
	for _, d := range existingDevices {
		existingDeviceIDs[d.DeviceID] = true
	}

	var newDevices []string
	for _, id := range deviceIDs {
		if !existingDeviceIDs[id] {
			newDevices = append(newDevices, id)
		}
	}

	newSelectedCount := (len(deviceIDs) * newPercentage) / 100
	currentCount := task.SelectedDevices
	needCount := newSelectedCount - currentCount

	if needCount > len(newDevices) {
		needCount = len(newDevices)
	}

	newSelectedDevices := m.selectRandomDevices(newDevices, needCount)

	fw, err := m.store.GetFirmware(task.FirmwareID)
	if err != nil {
		return err
	}

	for _, deviceID := range newSelectedDevices {
		df, _ := m.store.GetDeviceFirmware(deviceID)
		oldVersion := ""
		if df != nil {
			oldVersion = df.Version
		}

		taskDeviceID := uuid.New().String()
		taskDevice := &models.UpgradeTaskDevice{
			ID:         taskDeviceID,
			TaskID:     taskID,
			DeviceID:   deviceID,
			OldVersion: oldVersion,
			NewVersion: fw.Version,
			Status:     "pending",
			Progress:   0,
		}

		if err := m.store.CreateUpgradeTaskDevice(taskDevice); err != nil {
			log.Printf("Failed to create task device: %v", err)
			continue
		}

		m.notifyDeviceFirmware(deviceID, taskDeviceID, fw)
	}

	task.SelectedDevices = currentCount + needCount
	task.Percentage = newPercentage

	return m.store.UpdateUpgradeTask(task)
}

func (m *Manager) CancelTask(taskID string) error {
	task, err := m.store.GetUpgradeTask(taskID)
	if err != nil {
		return err
	}

	if task.Status != "in_progress" {
		return fmt.Errorf("task is not in progress")
	}

	now := time.Now()
	task.Status = "cancelled"
	task.CancelledAt = &now

	return m.store.UpdateUpgradeTask(task)
}

func (m *Manager) RetryFailedDevices(taskID string) error {
	task, err := m.store.GetUpgradeTask(taskID)
	if err != nil {
		return err
	}

	if task.Status != "in_progress" {
		return fmt.Errorf("task is not in progress")
	}

	devices, err := m.store.GetUpgradeTaskDevices(taskID)
	if err != nil {
		return err
	}

	fw, err := m.store.GetFirmware(task.FirmwareID)
	if err != nil {
		return err
	}

	for _, d := range devices {
		if d.Status == "failed" {
			d.Status = "pending"
			d.Progress = 0
			d.ErrorMsg = ""
			d.RetryCount++

			if err := m.store.UpdateUpgradeTaskDevice(d); err != nil {
				log.Printf("Failed to update task device: %v", err)
				continue
			}

			m.notifyDeviceFirmware(d.DeviceID, d.ID, fw)
		}
	}

	return nil
}

func (m *Manager) notifyDeviceFirmware(deviceID, taskDeviceID string, fw *models.Firmware) {
	msg := map[string]interface{}{
		"task_id":      taskDeviceID,
		"firmware_id":  fw.ID,
		"version":      fw.Version,
		"file_size":    fw.FileSize,
		"checksum":     fw.Checksum,
		"download_url": fmt.Sprintf("/firmware/%d/download", fw.ID),
	}

	payload, _ := json.Marshal(msg)
	topic := fmt.Sprintf("devices/%s/firmware/info", deviceID)
	m.mqttServer.Publish(topic, payload)
}

func (m *Manager) HandleDeviceStatus(deviceID string, payload []byte) error {
	var status struct {
		TaskID   string `json:"task_id"`
		Status   string `json:"status"`
		Progress int    `json:"progress"`
		Error    string `json:"error"`
	}

	if err := json.Unmarshal(payload, &status); err != nil {
		return err
	}

	td, err := m.store.GetUpgradeTaskDevice(status.TaskID)
	if err != nil {
		return nil
	}

	now := time.Now()
	td.Status = status.Status
	td.Progress = status.Progress

	if status.Status == "downloading" || status.Status == "installing" {
		td.StartedAt = &now
	}

	if status.Status == "success" {
		td.CompletedAt = &now
		m.updateTaskProgress(td.TaskID)
	} else if status.Status == "failed" {
		td.CompletedAt = &now
		td.ErrorMsg = status.Error
		m.updateTaskProgress(td.TaskID)
	}

	return m.store.UpdateUpgradeTaskDevice(td)
}

func (m *Manager) updateTaskProgress(taskDeviceID string) {
	td, err := m.store.GetUpgradeTaskDevice(taskDeviceID)
	if err != nil {
		return
	}

	task, err := m.store.GetUpgradeTask(td.TaskID)
	if err != nil {
		return
	}

	devices, _ := m.store.GetUpgradeTaskDevices(td.TaskID)

	successCount := 0
	failCount := 0
	allDone := true

	for _, d := range devices {
		if d.Status == "success" {
			successCount++
		} else if d.Status == "failed" {
			failCount++
		} else {
			allDone = false
		}
	}

	task.SuccessCount = successCount
	task.FailCount = failCount

	if allDone {
		task.Status = "completed"
		now := time.Now()
		task.CompletedAt = &now
	}

	m.store.UpdateUpgradeTask(task)
}

func (m *Manager) GetUpgradeTask(taskID string) (*models.UpgradeTask, []*models.UpgradeTaskDevice, error) {
	task, err := m.store.GetUpgradeTask(taskID)
	if err != nil {
		return nil, nil, err
	}

	devices, err := m.store.GetUpgradeTaskDevices(taskID)
	if err != nil {
		return nil, nil, err
	}

	return task, devices, nil
}

func (m *Manager) ListUpgradeTasks(limit, offset int) ([]*models.UpgradeTask, int64, error) {
	return m.store.ListUpgradeTasks(limit, offset)
}

func (m *Manager) GetUpgradeStatus(deviceID string) (*models.UpgradeStatusResponse, error) {
	devices, err := m.store.GetPendingUpgradeTaskDevices(deviceID)
	if err != nil || len(devices) == 0 {
		return nil, err
	}

	td := devices[0]
	return &models.UpgradeStatusResponse{
		TaskID:     td.TaskID,
		DeviceID:   td.DeviceID,
		Status:     td.Status,
		Progress:   td.Progress,
		ErrorMsg:   td.ErrorMsg,
		OldVersion: td.OldVersion,
		NewVersion: td.NewVersion,
	}, nil
}
