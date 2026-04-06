package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"iot-platform/internal/api"
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

	apiServer := api.NewServer(&cfg.Server, deviceMgr, mqttServer, store, wsHub)

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
