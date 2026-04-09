package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"iot-platform/internal/websocket"
	"iot-platform/pkg/models"
)

type Executor struct {
	wsHub      *websocket.Hub
	httpClient *http.Client
	mqttTopic  func(topic string, payload []byte) error
}

func NewExecutor(wsHub *websocket.Hub, mqttTopic func(topic string, payload []byte) error) *Executor {
	return &Executor{
		wsHub:      wsHub,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		mqttTopic:  mqttTopic,
	}
}

func (e *Executor) Execute(alert *models.Alert, rule *models.AlertRule, device *models.Device) error {
	actions, err := e.parseActions(rule.Actions)
	if err != nil {
		return err
	}

	sortActionsByPriority(actions)

	for _, action := range actions {
		if !action.Enabled {
			continue
		}

		switch action.Type {
		case "websocket":
			go e.ExecuteWebSocket(alert, action)
		case "webhook":
			go e.ExecuteWebhook(alert, rule, device, action)
		case "mqtt":
			go e.ExecuteMQTT(alert, rule, device, action)
		case "dashboard":
			go e.ExecuteWebSocket(alert, action)
		}
	}

	return nil
}

func (e *Executor) ExecuteWebSocket(alert *models.Alert, action *ActionConfig) error {
	if e.wsHub == nil {
		return nil
	}

	priorityStr := "low"
	switch alert.Priority {
	case models.PriorityHigh:
		priorityStr = "high"
	case models.PriorityMedium:
		priorityStr = "medium"
	}

	message := fmt.Sprintf("[%s优先级] %s", priorityStr, alert.Message)

	wsAlert := map[string]interface{}{
		"type": "alert",
		"data": map[string]interface{}{
			"id":            alert.ID,
			"rule_id":       alert.RuleID,
			"rule_name":     alert.RuleName,
			"device_id":     alert.DeviceID,
			"device_name":   alert.DeviceName,
			"status":        alert.Status,
			"priority":      priorityStr,
			"trigger_value": alert.TriggerValue,
			"message":       message,
			"created_at":    alert.CreatedAt.Format(time.RFC3339),
		},
	}

	e.wsHub.Broadcast(&websocket.Message{
		Type:    "alert",
		Payload: wsAlert,
	})

	return nil
}

func (e *Executor) ExecuteWebhook(alert *models.Alert, rule *models.AlertRule, device *models.Device, action *ActionConfig) error {
	if action.URL == "" {
		return nil
	}

	body := e.buildMessageTemplate(action.BodyTemplate, alert, rule, device)
	if body == nil {
		body = map[string]interface{}{
			"alert":     alert.RuleName,
			"device":    device.Name,
			"value":     alert.TriggerValue,
			"message":   alert.Message,
			"priority":  alert.Priority,
			"timestamp": time.Now().Format(time.RFC3339),
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(action.Method, action.URL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range action.Headers {
		if headerValue, ok := value.(string); ok {
			req.Header.Set(key, headerValue)
		}
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		log.Printf("Webhook execution failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (e *Executor) ExecuteMQTT(alert *models.Alert, rule *models.AlertRule, device *models.Device, action *ActionConfig) error {
	if e.mqttTopic == nil || action.Topic == "" {
		return nil
	}

	topic := e.buildTopicTemplate(action.Topic, alert, rule, device)
	message := e.buildMessageTemplate(action.MessageTemplate, alert, rule, device)
	if message == nil {
		message = map[string]interface{}{
			"alert":     alert.RuleName,
			"device":    device.Name,
			"value":     alert.TriggerValue,
			"message":   alert.Message,
			"priority":  alert.Priority,
			"timestamp": time.Now().Format(time.RFC3339),
		}
	}

	jsonMsg, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return e.mqttTopic(topic, jsonMsg)
}

func (e *Executor) buildMessageTemplate(template string, alert *models.Alert, rule *models.AlertRule, device *models.Device) interface{} {
	if template == "" {
		return nil
	}

	result := template
	result = replaceAll(result, "${rule.name}", alert.RuleName)
	result = replaceAll(result, "${device.id}", device.ID)
	result = replaceAll(result, "${device.name}", device.Name)
	result = replaceAll(result, "${value}", fmt.Sprintf("%v", alert.TriggerValue))
	result = replaceAll(result, "${message}", alert.Message)
	result = replaceAll(result, "${timestamp}", time.Now().Format(time.RFC3339))

	var resultMap map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resultMap); err != nil {
		return result
	}

	return resultMap
}

func (e *Executor) buildTopicTemplate(template string, alert *models.Alert, rule *models.AlertRule, device *models.Device) string {
	result := template
	result = replaceAll(result, "${device.id}", device.ID)
	result = replaceAll(result, "${rule.id}", rule.ID)
	result = replaceAll(result, "${user.id}", fmt.Sprintf("%d", alert.UserID))
	return result
}

func replaceAll(s, old, new string) string {
	for {
		idx := -1
		for i := 0; i <= len(s)-len(old); i++ {
			if s[i:i+len(old)] == old {
				idx = i
				break
			}
		}
		if idx == -1 {
			break
		}
		s = s[:idx] + new + s[idx+len(old):]
	}
	return s
}

type ActionConfig struct {
	Type            string                 `json:"type"`
	Enabled         bool                   `json:"enabled"`
	URL             string                 `json:"url,omitempty"`
	Method          string                 `json:"method,omitempty"`
	Headers         map[string]interface{} `json:"headers,omitempty"`
	BodyTemplate    string                 `json:"body_template,omitempty"`
	Topic           string                 `json:"topic,omitempty"`
	MessageTemplate string                 `json:"message_template,omitempty"`
	QOS             int                    `json:"qos,omitempty"`
	Retain          bool                   `json:"retain,omitempty"`
	Priority        string                 `json:"priority,omitempty"`
}

func (e *Executor) parseActions(actionsJSON string) ([]*ActionConfig, error) {
	var actions []*ActionConfig
	if err := json.Unmarshal([]byte(actionsJSON), &actions); err != nil {
		var singleAction *ActionConfig
		if err2 := json.Unmarshal([]byte(actionsJSON), &singleAction); err2 != nil {
			return nil, err
		}
		actions = append(actions, singleAction)
	}
	return actions, nil
}

func sortActionsByPriority(actions []*ActionConfig) {
	sort.Slice(actions, func(i, j int) bool {
		pi := getActionPriority(actions[i])
		pj := getActionPriority(actions[j])
		return pi > pj
	})
}

func getActionPriority(action *ActionConfig) int {
	switch action.Priority {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
