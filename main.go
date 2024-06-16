package main

import (
	"context"
	"log"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

const TopicConnectionStatus = "status"

func main() {

	if len(os.Args) != 2 {
		log.Fatalln("Usage: hass-tesla-vc <config.yaml>")
	}

	configData, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalln("Failed to read config file:", err)
	}

	var config Config

	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		log.Fatalln("Failed to unmarshal config file:", err)
	}

	cars := make(map[string]*Car)

	for _, cfg := range config.Cars {
		car, err := NewCar(&cfg, config.Mqtt.Prefix)
		if err != nil {
			log.Fatalln("Error setting up car:", err)
		}

		cars[cfg.ID] = car
		go car.ConnectCar(context.Background())
	}

	connectMqtt(&config.Mqtt, cars)

	select {}

}

func connectMqtt(cfg *MqttConfig, cars map[string]*Car) mqtt.Client {
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.URL).
		SetClientID("tesla_ble-"+cfg.Prefix).
		SetKeepAlive(2*time.Second).
		SetPingTimeout(1*time.Second).
		SetAutoReconnect(true).
		SetResumeSubs(true).
		SetOrderMatters(false).
		SetWill(cfg.Prefix+"/status", "offline", 0, true)

	opts.OnConnect = func(client mqtt.Client) {
		log.Println("MQTT connected")

		for _, car := range cars {
			car.SetupMqtt(client)
		}

		client.Publish(cfg.Prefix+"/status", 0, true, "online").Wait()
	}
	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Println("MQTT connection lost:", err)
	}

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalln("Failed to connect to MQTT server:", token.Error())
	}

	return c
}
