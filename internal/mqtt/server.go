package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"iot-platform/internal/config"
	"iot-platform/internal/device"
	"iot-platform/internal/storage"
	"iot-platform/pkg/models"
)

type Server struct {
	config      *config.MQTTConfig
	ln          net.Listener
	deviceMgr   *device.Manager
	storage     *storage.Store
	onMessage   func(*models.MQTTMessage)
	onTelemetry func(deviceID string, data map[string]interface{})
	conns       map[net.Conn]*mqttConn
	deviceConns map[string]net.Conn
	lock        sync.RWMutex
	running     bool
}

type mqttConn struct {
	conn          net.Conn
	deviceID      string
	authenticated bool
}

func NewServer(cfg *config.MQTTConfig, deviceMgr *device.Manager, store *storage.Store) *Server {
	return &Server{
		config:      cfg,
		deviceMgr:   deviceMgr,
		storage:     store,
		conns:       make(map[net.Conn]*mqttConn),
		deviceConns: make(map[string]net.Conn),
	}
}

func (s *Server) SetMessageCallback(cb func(*models.MQTTMessage)) {
	s.onMessage = cb
}

func (s *Server) SetTelemetryCallback(cb func(deviceID string, data map[string]interface{})) {
	s.onTelemetry = cb
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start MQTT broker: %w", err)
	}

	s.ln = ln
	s.running = true

	go s.acceptLoop()

	log.Printf("MQTT server started on %s (with auth)", addr)
	return nil
}

func (s *Server) acceptLoop() {
	for s.running {
		conn, err := s.ln.Accept()
		if err != nil {
			continue
		}

		s.lock.Lock()
		s.conns[conn] = &mqttConn{conn: conn}
		s.lock.Unlock()

		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() {
		conn.Close()
		s.lock.Lock()
		delete(s.conns, conn)
		for id, c := range s.deviceConns {
			if c == conn {
				delete(s.deviceConns, id)
				break
			}
		}
		s.lock.Unlock()
	}()

	buf := make([]byte, 4096)
	for {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		data := buf[:n]
		if len(data) > 0 {
			s.processPacket(conn, data)
		}
	}
}

func (s *Server) processPacket(conn net.Conn, data []byte) {
	if len(data) < 2 {
		return
	}

	packetType := data[0] & 0xF0

	switch packetType {
	case 0x10:
		s.handleConnect(conn, data[2:])
	case 0x30:
		s.handlePublish(conn, data[2:])
	case 0x82:
		s.handleSubscribe(conn, data[2:])
	case 0xE0:
		s.handleDisconnect(conn)
	}
}

func (s *Server) handleConnect(conn net.Conn, data []byte) {
	if len(data) < 12 {
		return
	}

	protocolLen := int(data[0])<<8 | int(data[1])
	if len(data) < 2+protocolLen+8 {
		return
	}

	protocol := string(data[2 : 2+protocolLen])
	if protocol != "MQTT" {
		return
	}

	offset := 2 + protocolLen
	version := data[offset]
	if version != 0x04 {
		return
	}

	offset++
	flags := data[offset]
	hasUsername := (flags & 0x80) != 0
	hasPassword := (flags & 0x40) != 0

	offset++
	offset += 2

	clientIDLen := int(data[offset])<<8 | int(data[offset+1])
	offset += 2
	clientID := string(data[offset : offset+clientIDLen])
	offset += clientIDLen

	var username, password string

	if hasUsername {
		usernameLen := int(data[offset])<<8 | int(data[offset+1])
		offset += 2
		username = string(data[offset : offset+usernameLen])
		offset += usernameLen
	}

	if hasPassword {
		passwordLen := int(data[offset])<<8 | int(data[offset+1])
		offset += 2
		password = string(data[offset : offset+passwordLen])
	}

	log.Printf("MQTT Connect: client=%s, username=%s", clientID, username)

	if !s.authenticate(username, password) {
		log.Printf("MQTT Auth failed for: %s", username)
		resp := []byte{0x20, 0x02, 0x00, 0x05}
		conn.Write(resp)
		return
	}

	s.lock.Lock()
	if mc, ok := s.conns[conn]; ok {
		mc.deviceID = username
		mc.authenticated = true
	}
	s.deviceConns[username] = conn
	s.lock.Unlock()

	resp := []byte{0x20, 0x02, 0x00, 0x00}
	conn.Write(resp)
	log.Printf("MQTT Auth success: %s", username)
}

func (s *Server) authenticate(username, password string) bool {
	if username == "" || password == "" {
		return false
	}

	device, err := s.deviceMgr.GetDevice(username)
	if err != nil {
		log.Printf("Device not found: %s", username)
		return false
	}

	if device.Disabled {
		log.Printf("Device disabled: %s", username)
		return false
	}

	return device.Secret == password
}

func (s *Server) DisconnectDevice(deviceID string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	conn, ok := s.deviceConns[deviceID]
	if !ok {
		return fmt.Errorf("device not connected")
	}

	conn.Write([]byte{0xE0, 0x00})
	conn.Close()
	delete(s.deviceConns, deviceID)
	delete(s.conns, conn)
	return nil
}

func (s *Server) handlePublish(conn net.Conn, data []byte) {
	s.lock.RLock()
	mc, ok := s.conns[conn]
	s.lock.RUnlock()

	if !ok || !mc.authenticated {
		return
	}

	payloadStart := -1
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == '{' {
			payloadStart = i
			break
		}
	}

	if payloadStart < 0 {
		return
	}

	topic := string(data[:payloadStart])
	payload := data[payloadStart:]

	log.Printf("MQTT Publish: topic=%s payload=%s", topic, string(payload))

	var deviceID string
	switch {
	case strings.HasSuffix(topic, "/telemetry"):
		deviceID = strings.Split(topic, "/")[1]
		s.handleTelemetry(deviceID, payload)
	case strings.HasSuffix(topic, "/heartbeat"):
		deviceID = strings.Split(topic, "/")[1]
		s.handleHeartbeat(deviceID, payload)
	case strings.HasSuffix(topic, "/status"):
		deviceID = strings.Split(topic, "/")[1]
		s.handleStatus(deviceID, payload)
	case strings.HasSuffix(topic, "/command/resp"):
		deviceID = strings.Split(topic, "/")[1]
		s.handleCommandResponse(deviceID, payload)
	}
}

func (s *Server) handleSubscribe(conn net.Conn, data []byte) {
	s.lock.RLock()
	mc, ok := s.conns[conn]
	s.lock.RUnlock()

	if !ok || !mc.authenticated {
		return
	}

	if len(data) < 2 {
		return
	}

	packetID := data[0]<<8 | data[1]
	resp := []byte{0x90, 0x03, byte(packetID >> 8), byte(packetID & 0xFF), 0x00}
	conn.Write(resp)
}

func (s *Server) handleDisconnect(conn net.Conn) {}

func (s *Server) handleTelemetry(deviceID string, payload []byte) {
	if err := s.storage.SaveTelemetry(deviceID, string(payload), time.Now()); err != nil {
		log.Printf("Failed to save telemetry: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err == nil {
		s.deviceMgr.UpdateProperties(deviceID, data)
	}

	s.deviceMgr.HandleHeartbeat(deviceID)

	if s.onMessage != nil {
		s.onMessage(&models.MQTTMessage{
			DeviceID:  deviceID,
			Topic:     "devices/" + deviceID + "/telemetry",
			Payload:   payload,
			Timestamp: time.Now(),
		})
	}

	if s.onTelemetry != nil {
		s.onTelemetry(deviceID, data)
	}
}

func (s *Server) handleHeartbeat(deviceID string, payload []byte) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err == nil {
		if status, ok := data["status"].(string); ok {
			s.deviceMgr.UpdateStatus(deviceID, models.DeviceStatus(status))
		}
		delete(data, "status")
		delete(data, "timestamp")
		if len(data) > 0 {
			s.deviceMgr.UpdateProperties(deviceID, data)
		}
	} else {
		s.deviceMgr.HandleHeartbeat(deviceID)
	}
}

func (s *Server) handleStatus(deviceID string, payload []byte) {
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(payload, &status); err == nil {
		s.deviceMgr.UpdateStatus(deviceID, models.DeviceStatus(status.Status))
	}
}

func (s *Server) handleCommandResponse(deviceID string, payload []byte) {
	var resp struct {
		CommandID string `json:"command_id"`
		Status    string `json:"status"`
		Result    string `json:"result"`
	}
	if err := json.Unmarshal(payload, &resp); err == nil {
		s.storage.UpdateCommandStatus(resp.CommandID, resp.Status, resp.Result)
	}
}

func (s *Server) PublishCommand(deviceID, commandID, command string, params map[string]interface{}) error {
	topic := fmt.Sprintf("devices/%s/command", deviceID)

	payload := map[string]interface{}{
		"command_id": commandID,
		"command":    command,
		"params":     params,
		"timestamp":  time.Now().Unix(),
	}

	data, _ := json.Marshal(payload)

	s.lock.Lock()
	defer s.lock.Unlock()

	for conn, mc := range s.conns {
		if mc.authenticated {
			packet := s.buildPublishPacket(topic, data)
			conn.Write(packet)
		}
	}

	return nil
}

func (s *Server) buildPublishPacket(topic string, payload []byte) []byte {
	topicLen := len(topic)
	totalLen := 2 + topicLen + len(payload)

	packet := make([]byte, 2+totalLen)
	packet[0] = 0x30
	packet[1] = byte(totalLen)
	packet[2] = byte(topicLen >> 8)
	packet[3] = byte(topicLen & 0xFF)
	copy(packet[4:], topic)
	copy(packet[4+topicLen:], payload)

	return packet
}

func (s *Server) Publish(topic string, payload []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	for conn, mc := range s.conns {
		if mc.authenticated {
			packet := s.buildPublishPacket(topic, payload)
			conn.Write(packet)
		}
	}

	return nil
}

func (s *Server) Stop() {
	s.running = false
	if s.ln != nil {
		s.ln.Close()
	}

	s.lock.Lock()
	for conn := range s.conns {
		conn.Close()
	}
	s.lock.Unlock()
}
