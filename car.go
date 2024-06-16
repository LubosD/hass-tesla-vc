package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/teslamotors/vehicle-command/pkg/connector/ble"
	"github.com/teslamotors/vehicle-command/pkg/protocol"
	"github.com/teslamotors/vehicle-command/pkg/protocol/protobuf/universalmessage"
	"github.com/teslamotors/vehicle-command/pkg/vehicle"
)

const MAX_ATTEMPTS = 2

type CarCommand struct {
	Attempts int
	Op       func(ctx context.Context, v *vehicle.Vehicle) error
}

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
		commands: make(chan CarCommand, 20),
		prefix:   prefix,
	}, nil
}

func (c *Car) ID() string {
	return c.config.ID
}

func (c *Car) IsConnected() bool {
	return c.connected
}

func (c *Car) ConnectCar(ctx context.Context) {
	for {
		// Wait for the next command
		log.Println("Waiting for next command for VIN " + c.config.VIN)
		cmd := <-c.commands

		log.Println("Trying to connect to VIN " + c.config.VIN)
		conn, err := ble.NewConnection(ctx, c.config.VIN)
		if err == nil {
			err = c.operateConnection(ctx, conn, cmd)

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

func (c *Car) operateConnection(ctx context.Context, conn *ble.Connection, firstCommand CarCommand) error {
	log.Println("VIN " + c.config.VIN + " connected over BLE!")

	defer func() {
		log.Println("VIN " + c.config.VIN + " disconnected")
		c.connected = false
		// c.PublishStatus()
	}()

	car, err := vehicle.NewVehicle(conn, c.skey, nil)
	if err != nil {
		return err
	}

	err = car.Connect(ctx)
	if err != nil {
		return err
	}

	// First connect just VCSEC so we can Wakeup() the car if needed.
	err = car.StartSession(ctx, []universalmessage.Domain{
		protocol.DomainVCSEC,
	})
	if err != nil {
		return err
	}

	err = car.Wakeup(ctx)
	if err != nil {
		return err
	}

	// Then we can also connect the infotainment
	err = car.StartSession(ctx, []universalmessage.Domain{
		protocol.DomainVCSEC,
		protocol.DomainInfotainment,
	})
	if err != nil {
		return fmt.Errorf("can't create session (is key added to the car?): %w", err)
	}

	log.Println("Session started")
	c.connected = true
	// c.PublishStatus()

	// t := time.NewTicker(time.Second * 5)

	err = c.executeCommand(car, ctx, &firstCommand)
	if err != nil {
		return err
	}

	for {
		select {
		// Disabled for now. It doesn't seem to prevent BLE timeouts on RPi :-(
		//case <-t.C:
		//err := car.Ping(ctx)
		//if err != nil {
		//	return fmt.Errorf("ping failed: %w", err)
		//}
		//err = car.SetChargingAmps(ctx, 5)
		//	if err != nil {
		//		return err
		//	}
		//	log.Println("PING OK")
		case cmd, ok := <-c.commands:
			if !ok {
				return nil
			}

			err := c.executeCommand(car, ctx, &cmd)
			if err != nil {
				return err
			}
		}
	}
}

func (c *Car) executeCommand(car *vehicle.Vehicle, ctx context.Context, cmd *CarCommand) error {
	err := cmd.Op(ctx, car)
	if err != nil {
		cmd.Attempts++

		if cmd.Attempts < MAX_ATTEMPTS {
			c.commands <- *cmd
		}
		return err
	}

	return nil
}

func (c *Car) TopicNameForValue(valueName string) string {
	return c.prefix + "/" + c.config.ID + "/" + valueName
}

func intPtr(value int) *int {
	return &value
}

func (car *Car) SetupMqtt(client mqtt.Client) {
	car.mqttClient = client
	client.Subscribe(car.TopicNameForValue("charging_amps_set"), 0, func(c mqtt.Client, m mqtt.Message) {
		valueStr := string(m.Payload())
		valueFloat, err := strconv.ParseFloat(valueStr, 32)

		defer m.Ack()

		if err != nil {
			log.Println("Invalid charging_amps:", err)
			return
		}

		car.PushCommand(CarCommand{
			Op: func(ctx context.Context, v *vehicle.Vehicle) error {
				valueInt := int32(math.Round(valueFloat))
				err := v.SetChargingAmps(ctx, valueInt)

				if err == nil {
					log.Printf("SetChargingAmps to %d OK\n", valueInt)
					car.mqttClient.Publish(car.TopicNameForValue("charging_amps"), 0, true, strconv.Itoa(int(valueInt))).Wait()
				}

				return err
			},
		})
	})

	var autoconf HassAutoconfig
	autoconf.Name = "charging_amps"
	autoconf.StatusTopic = car.TopicNameForValue("charging_amps")
	autoconf.CommandTopic = car.TopicNameForValue("charging_amps_set")
	autoconf.UniqueID = autoconf.StatusTopic
	autoconf.DeviceClass = "current"
	autoconf.Max = intPtr(16)
	autoconf.Min = intPtr(0)
	autoconf.Device.IDs = car.prefix + "_" + car.ID()
	autoconf.Device.Name = car.ID()
	jsonBytes, _ := json.Marshal(&autoconf)

	client.Publish("homeassistant/number/"+car.prefix+"_"+car.ID()+"/"+autoconf.Name+"/config", 0, true, string(jsonBytes)).Wait()

	// car.PublishStatus()
}
