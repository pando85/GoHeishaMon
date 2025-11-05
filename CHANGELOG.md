# Changelog

## 1.2.0

- Refactor logging system to use different log levels
- Rewrite serial connection handling to improve stability
- Fix Operating_Mode_State and SetOperationMode
- Set powerful and quiet modes independently

## 1.1.1

- Add init script to start GoHeishaMon on boot
- Fix stack overflow in read serial goroutines
- Add buttons functionality to recover connection and original firmware restore process
  documentation
- Add status topic to MQTT

## 1.1.0

- Add TLS support to MQTT
- Change config path to `/etc/config/goheishamon.toml`
- Add install target in Makefile for upgrade process
- Implement a buffer for serial communication to fix wrong header error

## 1.0.192

- Use LEDs to monitor GoHeishaMon process and reset button to restart it.

## 1.0.191

- Tested in CZ-TAW1B
- Kernel updated

## 1.0.166

- Add new topics

## 1.0.159

- Removed a2wmain watch
- Start ssh and www from script
- Home Assistant MQTT Discovery https://www.home-assistant.io/docs/mqtt/discovery/

## 1.0.150

- Moved buttons handling from GoHeishaMon to separate script ( in this way , if GoHeishaMon will
  crash it is still possible to go back to orginal via 3 buttons)
