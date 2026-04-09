package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"iot-platform/internal/alert"
	"iot-platform/internal/api"
	"iot-platform/internal/auth"
	"iot-platform/internal/config"
	"iot-platform/internal/device"
	"iot-platform/internal/mqtt"
	"iot-platform/internal/storage"
	"iot-platform/internal/websocket"
	"iot-platform/pkg/models"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	store, err := storage.New(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	if err := initDefaultAdmin(store, cfg); err != nil {
		log.Printf("Warning: failed to create default admin: %v", err)
	}

	deviceMgr := device.NewManager(store.DB(), store)
	if err := deviceMgr.LoadFromDB(); err != nil {
		log.Fatalf("Failed to load devices: %v", err)
	}

	wsHub := websocket.NewHub()
	go wsHub.Run()

	mqttServer := mqtt.NewServer(&cfg.MQTT, deviceMgr, store)
	mqttServer.SetMessageCallback(func(msg *models.MQTTMessage) {
		wsHub.Broadcast(&websocket.Message{
			Type:    "mqtt_message",
			Payload: msg,
		})
	})

	if err := mqttServer.Start(); err != nil {
		log.Fatalf("Failed to start MQTT server: %v", err)
	}

	deviceMgr.SetUpdateCallback(func(d *models.Device) {
		wsHub.Broadcast(&websocket.Message{
			Type:    "device_updated",
			Payload: d,
		})
	})

	alertStore := alert.NewAlertStore(store.DB())
	alertEvaluator := alert.NewEvaluator()
	alertExecutor := alert.NewExecutor(wsHub, func(topic string, payload []byte) error {
		return mqttServer.Publish(topic, payload)
	})
	alertManager := alert.NewManager(alertStore, alertEvaluator, alertExecutor)
	if err := alertManager.Initialize(); err != nil {
		log.Fatalf("Failed to initialize alert manager: %v", err)
	}

	mqttServer.SetTelemetryCallback(func(deviceID string, data map[string]interface{}) {
		alertManager.ProcessTelemetry(deviceID, data)
	})

	alertHandler := alert.NewHandler(alertManager)

	apiServer := api.NewServer(cfg, deviceMgr, mqttServer, store, wsHub, alertHandler)

	go func() {
		log.Printf("Starting HTTP API server on port %s", cfg.Server.HTTPAddr)
		if err := apiServer.Start(); err != nil {
			log.Fatalf("Failed to start API server: %v", err)
		}
	}()

	log.Printf("IoT Platform started successfully")
	log.Printf("HTTP API: http://localhost:%s", cfg.Server.HTTPAddr)
	log.Printf("WebSocket: ws://localhost:%s/ws", cfg.Server.WSAddr)
	log.Printf("MQTT: tcp://localhost:%d", cfg.MQTT.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	mqttServer.Stop()

	log.Println("Goodbye!")
}

func initDefaultAdmin(store *storage.Store, cfg *config.Config) error {
	username := cfg.Auth.DefaultAdmin
	if username == "" {
		return nil
	}

	existing, _ := store.GetUserByUsername(username)
	if existing != nil {
		return nil
	}

	passwordHash, err := auth.HashPassword(cfg.Auth.DefaultPassword)
	if err != nil {
		return err
	}

	user := &models.User{
		Username:     username,
		PasswordHash: passwordHash,
		Role:         "admin",
	}

	return store.CreateUser(user)
}
