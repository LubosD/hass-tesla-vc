# MQTT Tesla control over BLE

This small project allows for controlling Tesla cars over BLE. Right now, only setting the charging current is possible.

## Setup

* Use [tesla-control](https://github.com/teslamotors/vehicle-command/blob/main/cmd/tesla-control/README.md) to create a key pair and enroll it in your vehicle.
* Important: don't let it save the private key into a keychain! This project cannot work with keychains. Use `-key-file`.
* Configure the vehicle(s) based on the [example.yaml](example.yaml).
* Build the executable and grant BLE permissions to it:

```
go build
sudo setcap 'cap_net_admin=eip' hass-tesla-vc
```
* Run:

```
./hass-tesla-vc config.yaml
```
