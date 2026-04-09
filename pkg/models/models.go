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

type User struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Username     string    `json:"username" gorm:"uniqueIndex;type:varchar(50);not null"`
	PasswordHash string    `json:"-" gorm:"type:varchar(255);not null"`
	Role         string    `json:"role" gorm:"type:varchar(20);default:'user'"`
	Disabled     bool      `json:"disabled" gorm:"type:bool;default:false"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type RefreshToken struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"index"`
	Token     string    `json:"token" gorm:"uniqueIndex;type:varchar(64)"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type Device struct {
	ID         string       `json:"id" gorm:"primaryKey;type:varchar(36)"`
	Name       string       `json:"name" gorm:"type:varchar(100);not null"`
	TypeID     uint         `json:"type_id" gorm:"type:integer"`
	UserID     uint         `json:"user_id" gorm:"index"`
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
	UserID     uint   `json:"user_id"`
	OwnerName  string `json:"owner_name"`
	Status     string `json:"status"`
	Secret     string `json:"secret"`
	Disabled   bool   `json:"disabled"`
	Properties string `json:"properties"`
	LastSeen   string `json:"last_seen"`
	CreatedAt  string `json:"created_at"`
}

type UserResponse struct {
	ID        uint      `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

type LoginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	TokenType    string       `json:"token_type"`
	User         UserResponse `json:"user"`
}

type RefreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type AlertPriority int

const (
	PriorityLow    AlertPriority = 1
	PriorityMedium AlertPriority = 2
	PriorityHigh   AlertPriority = 3
)

type AlertRule struct {
	ID            string        `json:"id" gorm:"primaryKey;type:varchar(36)"`
	Name          string        `json:"name" gorm:"type:varchar(100);not null"`
	Description   string        `json:"description" gorm:"type:text"`
	UserID        uint          `json:"user_id" gorm:"index"`
	DeviceID      string        `json:"device_id" gorm:"type:varchar(36);index"`
	DeviceType    string        `json:"device_type" gorm:"type:varchar(50)"`
	ConditionType string        `json:"condition_type" gorm:"type:varchar(20);not null"`
	Conditions    string        `json:"conditions" gorm:"type:text;not null"`
	Expression    string        `json:"expression" gorm:"type:text"`
	Actions       string        `json:"actions" gorm:"type:text;not null"`
	Priority      AlertPriority `json:"priority" gorm:"type:integer;default:2"`
	Enabled       bool          `json:"enabled" gorm:"type:bool;default:true"`
	CheckInterval int           `json:"check_interval" gorm:"type:integer;default:60"`
	Cooldown      int           `json:"cooldown" gorm:"type:integer;default:300"`
	LastTriggered *time.Time    `json:"last_triggered"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

type Alert struct {
	ID             string        `json:"id" gorm:"primaryKey;type:varchar(36)"`
	RuleID         string        `json:"rule_id" gorm:"type:varchar(36);index"`
	RuleName       string        `json:"rule_name"`
	DeviceID       string        `json:"device_id" gorm:"type:varchar(36);index"`
	DeviceName     string        `json:"device_name"`
	UserID         uint          `json:"user_id" gorm:"index"`
	Status         string        `json:"status" gorm:"type:varchar(20);default:'active'"`
	Priority       AlertPriority `json:"priority"`
	TriggerValue   string        `json:"trigger_value" gorm:"type:text"`
	Message        string        `json:"message" gorm:"type:text"`
	AcknowledgedAt *time.Time    `json:"acknowledged_at"`
	AcknowledgedBy *uint         `json:"acknowledged_by"`
	ResolvedAt     *time.Time    `json:"resolved_at"`
	CreatedAt      time.Time     `json:"created_at"`
}

type AlertRuleRequest struct {
	Name          string                 `json:"name" binding:"required"`
	Description   string                 `json:"description"`
	DeviceID      string                 `json:"device_id"`
	DeviceType    string                 `json:"device_type"`
	ConditionType string                 `json:"condition_type" binding:"required"`
	Conditions    map[string]interface{} `json:"conditions" binding:"required"`
	Expression    string                 `json:"expression"`
	Actions       map[string]interface{} `json:"actions" binding:"required"`
	Priority      AlertPriority          `json:"priority"`
	Enabled       bool                   `json:"enabled"`
	CheckInterval int                    `json:"check_interval"`
	Cooldown      int                    `json:"cooldown"`
}

type AlertRuleResponse struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	DeviceID      string        `json:"device_id"`
	DeviceType    string        `json:"device_type"`
	ConditionType string        `json:"condition_type"`
	Conditions    string        `json:"conditions"`
	Expression    string        `json:"expression"`
	Actions       string        `json:"actions"`
	Priority      AlertPriority `json:"priority"`
	Enabled       bool          `json:"enabled"`
	CheckInterval int           `json:"check_interval"`
	Cooldown      int           `json:"cooldown"`
	LastTriggered *time.Time    `json:"last_triggered"`
	CreatedAt     time.Time     `json:"created_at"`
}

type AlertResponse struct {
	ID             string        `json:"id"`
	RuleID         string        `json:"rule_id"`
	RuleName       string        `json:"rule_name"`
	DeviceID       string        `json:"device_id"`
	DeviceName     string        `json:"device_name"`
	UserID         uint          `json:"user_id"`
	Status         string        `json:"status"`
	Priority       AlertPriority `json:"priority"`
	TriggerValue   string        `json:"trigger_value"`
	Message        string        `json:"message"`
	AcknowledgedAt *time.Time    `json:"acknowledged_at"`
	AcknowledgedBy *uint         `json:"acknowledged_by"`
	ResolvedAt     *time.Time    `json:"resolved_at"`
	CreatedAt      time.Time     `json:"created_at"`
}

type AlertStats struct {
	ActiveCount       int64 `json:"active_count"`
	AcknowledgedCount int64 `json:"acknowledged_count"`
	TodayCount        int64 `json:"today_count"`
}
