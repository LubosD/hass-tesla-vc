package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/teslamotors/vehicle-command/pkg/connector/ble"
	"github.com/teslamotors/vehicle-command/pkg/protocol"
	"github.com/teslamotors/vehicle-command/pkg/protocol/protobuf/universalmessage"
	"github.com/teslamotors/vehicle-command/pkg/vehicle"
)

type CarCommand func(ctx context.Context, v *vehicle.Vehicle) error

type Car struct {
	config *CarConfig
	skey   protocol.ECDHPrivateKey

	connected bool
	commands  chan CarCommand

	prefix     string
	mqttClient mqtt.Client
}

func NewCar(config *CarConfig, prefix string) (*Car, error) {
	skey, err := protocol.LoadPrivateKey(config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	return &Car{
		config:   config,
		skey:     skey,
		commands: make(chan CarCommand),
		prefix:   prefix,
	}, nil
}

func (c *Car) ID() string {
	return c.config.ID
}

func (c *Car) IsConnected() bool {
	return c.connected
}

func (c *Car) ConnectLoop(ctx context.Context) {
	for {
		log.Println("Trying to connect to VIN " + c.config.VIN)
		conn, err := ble.NewConnection(ctx, c.config.VIN)
		if err == nil {
			err = c.operateConnection(ctx, conn)

			log.Println("OperateConnection returned:", err)
			conn.Close()
		}

		log.Println("Connect failed:", err)
		time.Sleep(time.Second)
	}
}

func (c *Car) PushCommand(cmd CarCommand) {
	c.commands <- cmd
}

func (c *Car) Shutdown() {
	close(c.commands)
}

func (c *Car) PublishStatus() {
	if c.mqttClient == nil {
		return
	}

	statusStr := "online"
	if !c.connected {
		statusStr = "offline"
	}

	c.mqttClient.Publish(c.TopicNameForValue(TopicConnectionStatus), 0, true, statusStr).Wait()
}

func (c *Car) operateConnection(ctx context.Context, conn *ble.Connection) error {
	log.Println("VIN " + c.config.VIN + " connected over BLE!")

	defer func() {
		log.Println("VIN " + c.config.VIN + " disconnected")
		c.connected = false
		c.PublishStatus()
	}()

	car, err := vehicle.NewVehicle(conn, c.skey, nil)
	if err != nil {
		return err
	}

	err = car.Connect(ctx)
	if err != nil {
		return err
	}

	err = car.StartSession(ctx, []universalmessage.Domain{
		protocol.DomainVCSEC,
		protocol.DomainInfotainment,
	})
	if err != nil {
		return fmt.Errorf("can't create session (is key added to the car?): %w", err)
	}

	log.Println("Session started")
	c.connected = true
	c.PublishStatus()

	car.PrivateKeyAvailable()

	t := time.NewTicker(time.Second * 5)

	for {
		select {
		case <-t.C:
			//err := car.Ping(ctx)
			//if err != nil {
			//	return fmt.Errorf("ping failed: %w", err)
			//}
			// Ping seems to require the infotainment subsystem,
			// which we don't want to activate.
			err = car.Ping(ctx)
			if err != nil {
				return err
			}
			log.Println("PING OK")
		case cmd, ok := <-c.commands:
			if !ok {
				return nil
			}

			err := cmd(ctx, car)
			if err != nil {
				return err
			}
		}
	}
}

func (c *Car) TopicNameForValue(valueName string) string {
	return c.prefix + "/" + c.config.ID + "/" + valueName
}

func intPtr(value int) *int {
	return &value
}

func (car *Car) SetupMqtt(client mqtt.Client) {
	car.mqttClient = client
	client.Subscribe(car.TopicNameForValue("charging_amps"), 0, func(c mqtt.Client, m mqtt.Message) {
		valueStr := string(m.Payload())
		valueInt, err := strconv.Atoi(valueStr)

		defer m.Ack()

		if err != nil {
			log.Println("Invalid charging_amps:", err)
			return
		}

		car.PushCommand(func(ctx context.Context, v *vehicle.Vehicle) error {
			return v.SetChargingAmps(ctx, int32(valueInt))
		})
	})

	var autoconf HassAutoconfig
	autoconf.Name = "charging_amps"
	autoconf.StatusTopic = car.TopicNameForValue("charging_amps")
	autoconf.CommandTopic = autoconf.StatusTopic
	autoconf.UniqueID = autoconf.StatusTopic
	autoconf.Max = intPtr(16)
	autoconf.Min = intPtr(0)
	jsonBytes, _ := json.Marshal(&autoconf)

	client.Publish("homeassistant/number/"+car.prefix+"_"+car.ID()+"/"+autoconf.Name+"/config", 0, true, string(jsonBytes)).Wait()

	car.PublishStatus()
}
