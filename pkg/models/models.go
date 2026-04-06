package models

import (
	"encoding/json"
	"time"
)

type DeviceStatus string

const (
	StatusOnline  DeviceStatus = "online"
	StatusOffline DeviceStatus = "offline"
)

type Device struct {
	ID         string       `json:"id" gorm:"primaryKey;type:varchar(36)"`
	Name       string       `json:"name" gorm:"type:varchar(100);not null"`
	TypeID     uint         `json:"type_id" gorm:"type:integer"`
	Status     DeviceStatus `json:"status" gorm:"type:varchar(20);default:'offline'"`
	Secret     string       `json:"secret" gorm:"type:varchar(64)"`
	Disabled   bool         `json:"disabled" gorm:"type:bool;default:false"`
	LastSeen   time.Time    `json:"last_seen"`
	Properties string       `json:"properties" gorm:"type:text"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

type TelemetryData struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	DeviceID  string    `json:"device_id" gorm:"type:varchar(36);index"`
	Data      string    `json:"data" gorm:"type:text"`
	Timestamp time.Time `json:"timestamp"`
}

type DeviceCommand struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	DeviceID  string    `json:"device_id" gorm:"type:varchar(36);index"`
	Command   string    `json:"command" gorm:"type:varchar(50)"`
	Params    string    `json:"params" gorm:"type:text"`
	Status    string    `json:"status" gorm:"type:varchar(20);default:'pending'"`
	Result    string    `json:"result" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DeviceType struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name      string    `json:"name" gorm:"type:varchar(50);unique;not null"`
	CreatedAt time.Time `json:"created_at"`
}

type MQTTMessage struct {
	DeviceID  string    `json:"device_id"`
	Topic     string    `json:"topic"`
	Payload   []byte    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

type Properties map[string]interface{}

func (d *Device) GetProperties() (Properties, error) {
	if d.Properties == "" {
		return make(Properties), nil
	}
	var props Properties
	err := json.Unmarshal([]byte(d.Properties), &props)
	return props, err
}

func (d *Device) SetProperties(props Properties) error {
	if props == nil {
		d.Properties = "{}"
		return nil
	}
	data, err := json.Marshal(props)
	if err != nil {
		return err
	}
	d.Properties = string(data)
	return nil
}

type DeviceResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	TypeID     uint   `json:"type_id"`
	TypeName   string `json:"type_name"`
	Status     string `json:"status"`
	Secret     string `json:"secret"`
	Disabled   bool   `json:"disabled"`
	Properties string `json:"properties"`
	LastSeen   string `json:"last_seen"`
	CreatedAt  string `json:"created_at"`
}
