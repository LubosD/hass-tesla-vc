package main

type Config struct {
	Mqtt MqttConfig `yaml:"mqtt"`

	Cars []CarConfig `yaml:"cars"`
}

type CarConfig struct {
	ID      string `yaml:"id"`
	VIN     string `yaml:"vin"`
	KeyFile string `yaml:"keyFile"`
}

type MqttConfig struct {
	// e.g. tcp://127.0.0.1:1883
	URL string `yaml:"url"`

	// e.g. "tesla-ble"
	Prefix string `yaml:"prefix"`
}
