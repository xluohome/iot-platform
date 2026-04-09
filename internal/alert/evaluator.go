package alert

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"iot-platform/pkg/models"

	"github.com/expr-lang/expr"
)

type Evaluator struct{}

func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

func (e *Evaluator) Evaluate(rule *models.AlertRule, device *models.Device, data map[string]interface{}) (bool, interface{}, error) {
	switch rule.ConditionType {
	case "threshold":
		return e.EvaluateThreshold(rule, device, data)
	case "status":
		return e.EvaluateStatus(rule, device)
	case "change":
		return e.EvaluateChange(rule, device, data)
	case "expression":
		return e.EvaluateExpression(rule, device, data)
	default:
		return false, nil, fmt.Errorf("unknown condition type: %s", rule.ConditionType)
	}
}

func (e *Evaluator) EvaluateThreshold(rule *models.AlertRule, device *models.Device, data map[string]interface{}) (bool, interface{}, error) {
	var condition struct {
		Field    string      `json:"field"`
		Operator string      `json:"operator"`
		Value    interface{} `json:"value"`
	}

	if err := json.Unmarshal([]byte(rule.Conditions), &condition); err != nil {
		return false, nil, err
	}

	fieldValue := e.getFieldValue(data, condition.Field)
	if fieldValue == nil {
		return false, nil, nil
	}

	triggered, err := e.compareValues(fmt.Sprintf("%v", fieldValue), condition.Operator, condition.Value)
	if err != nil {
		return false, nil, err
	}

	return triggered, fieldValue, nil
}

func (e *Evaluator) EvaluateStatus(rule *models.AlertRule, device *models.Device) (bool, interface{}, error) {
	var condition struct {
		Status   string `json:"status"`
		Duration int    `json:"duration"`
	}

	if err := json.Unmarshal([]byte(rule.Conditions), &condition); err != nil {
		return false, nil, err
	}

	if string(device.Status) != condition.Status {
		return false, nil, nil
	}

	if condition.Duration > 0 {
		offlineDuration := time.Since(device.LastSeen).Seconds()
		if offlineDuration < float64(condition.Duration) {
			return false, nil, nil
		}
	}

	return true, string(device.Status), nil
}

func (e *Evaluator) EvaluateChange(rule *models.AlertRule, device *models.Device, data map[string]interface{}) (bool, interface{}, error) {
	var condition struct {
		Field         string      `json:"field"`
		PreviousValue interface{} `json:"previous_value"`
	}

	if err := json.Unmarshal([]byte(rule.Conditions), &condition); err != nil {
		return false, nil, err
	}

	fieldValue := e.getFieldValue(data, condition.Field)
	if fieldValue == nil {
		return false, nil, nil
	}

	currentStr := fmt.Sprintf("%v", fieldValue)
	previousStr := fmt.Sprintf("%v", condition.PreviousValue)

	return currentStr != previousStr, fieldValue, nil
}

func (e *Evaluator) EvaluateExpression(rule *models.AlertRule, device *models.Device, data map[string]interface{}) (bool, interface{}, error) {
	if rule.Expression == "" {
		return false, nil, fmt.Errorf("expression is empty")
	}

	env := map[string]interface{}{
		"device": map[string]interface{}{
			"id":         device.ID,
			"name":       device.Name,
			"status":     string(device.Status),
			"properties": data,
		},
	}

	for key, value := range data {
		env[key] = value
	}

	result, err := expr.Eval(rule.Expression, env)
	if err != nil {
		return false, nil, err
	}

	triggered, ok := result.(bool)
	if !ok {
		return false, nil, fmt.Errorf("expression must return boolean")
	}

	return triggered, data, nil
}

func (e *Evaluator) getFieldValue(data map[string]interface{}, field string) interface{} {
	parts := strings.Split(field, ".")

	if len(parts) == 1 {
		return data[field]
	}

	if parts[0] == "properties" || parts[0] == "device" {
		var current interface{} = data
		for i := 1; i < len(parts); i++ {
			if currentMap, ok := current.(map[string]interface{}); ok {
				current = currentMap[parts[i]]
			} else {
				return nil
			}
		}
		return current
	}

	return data[field]
}

func (e *Evaluator) compareValues(actual, operator string, expected interface{}) (bool, error) {
	switch operator {
	case ">":
		return e.compareGreaterThan(actual, expected)
	case "<":
		return e.compareLessThan(actual, expected)
	case ">=":
		return e.compareGreaterOrEqual(actual, expected)
	case "<=":
		return e.compareLessOrEqual(actual, expected)
	case "==":
		return e.compareEqual(actual, expected)
	case "!=":
		return e.compareNotEqual(actual, expected)
	case "contains":
		return strings.Contains(actual, fmt.Sprintf("%v", expected)), nil
	case "startsWith":
		return strings.HasPrefix(actual, fmt.Sprintf("%v", expected)), nil
	case "endsWith":
		return strings.HasSuffix(actual, fmt.Sprintf("%v", expected)), nil
	case "matches":
		matched, err := regexp.MatchString(fmt.Sprintf("%v", expected), actual)
		return matched, err
	default:
		return false, fmt.Errorf("unknown operator: %s", operator)
	}
}

func (e *Evaluator) compareGreaterThan(actual string, expected interface{}) (bool, error) {
	actualNum, err := strconv.ParseFloat(actual, 64)
	if err != nil {
		return strings.Compare(actual, fmt.Sprintf("%v", expected)) > 0, nil
	}

	switch v := expected.(type) {
	case float64:
		return actualNum > v, nil
	case int:
		return actualNum > float64(v), nil
	case string:
		expectedNum, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return false, err
		}
		return actualNum > expectedNum, nil
	default:
		return false, fmt.Errorf("cannot compare number with %s", reflect.TypeOf(expected))
	}
}

func (e *Evaluator) compareLessThan(actual string, expected interface{}) (bool, error) {
	actualNum, err := strconv.ParseFloat(actual, 64)
	if err != nil {
		return strings.Compare(actual, fmt.Sprintf("%v", expected)) < 0, nil
	}

	switch v := expected.(type) {
	case float64:
		return actualNum < v, nil
	case int:
		return actualNum < float64(v), nil
	case string:
		expectedNum, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return false, err
		}
		return actualNum < expectedNum, nil
	default:
		return false, fmt.Errorf("cannot compare number with %s", reflect.TypeOf(expected))
	}
}

func (e *Evaluator) compareGreaterOrEqual(actual string, expected interface{}) (bool, error) {
	greater, err := e.compareGreaterThan(actual, expected)
	if err != nil {
		return false, err
	}
	if greater {
		return true, nil
	}
	return e.compareEqual(actual, expected)
}

func (e *Evaluator) compareLessOrEqual(actual string, expected interface{}) (bool, error) {
	less, err := e.compareLessThan(actual, expected)
	if err != nil {
		return false, err
	}
	if less {
		return true, nil
	}
	return e.compareEqual(actual, expected)
}

func (e *Evaluator) compareEqual(actual string, expected interface{}) (bool, error) {
	actualNum, err := strconv.ParseFloat(actual, 64)
	if err == nil {
		switch v := expected.(type) {
		case float64:
			return actualNum == v, nil
		case int:
			return actualNum == float64(v), nil
		case string:
			expectedNum, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return actual == v, nil
			}
			return actualNum == expectedNum, nil
		}
	}
	return actual == fmt.Sprintf("%v", expected), nil
}

func (e *Evaluator) compareNotEqual(actual string, expected interface{}) (bool, error) {
	equal, err := e.compareEqual(actual, expected)
	if err != nil {
		return true, nil
	}
	return !equal, nil
}
