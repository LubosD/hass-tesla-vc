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
		go car.ConnectLoop(context.Background())

		mqttClient := connectMqtt(&config.Mqtt, car)
		car.SetupMqtt(mqttClient)
	}

	select {}

}

func connectMqtt(cfg *MqttConfig, car *Car) mqtt.Client {
	opts := mqtt.NewClientOptions().AddBroker(cfg.URL).SetClientID("tesla_ble-" + car.ID())

	opts.SetKeepAlive(2 * time.Second)
	opts.SetPingTimeout(1 * time.Second)
	opts.SetWill(car.TopicNameForValue(TopicConnectionStatus), "offline", 0, true)
	opts.OnConnect = func(client mqtt.Client) {
		car.PublishStatus()
	}

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalln("Failed to connect to MQTT server:", token.Error())
	}

	return c
}
