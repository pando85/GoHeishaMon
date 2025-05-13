#!/bin/sh

WIFI_IF="wlan0"
PING_HOST=$(ip route | awk '/^default/ {print $3; exit}')
FAIL_COUNT_FILE="/tmp/wifi_fail_count"
MAX_FAILS=15 # 15 minutes (1 check every minute)

fail_count=0
[ -f "$FAIL_COUNT_FILE" ] && fail_count=$(cat "$FAIL_COUNT_FILE")

if ping -I "$WIFI_IF" -c 1 -W 5 "$PING_HOST" >/dev/null 2>&1; then
    echo 0 > "$FAIL_COUNT_FILE"
else
    fail_count=$((fail_count + 1))
    echo "$fail_count" > "$FAIL_COUNT_FILE"
    if [ "$fail_count" -ge "$MAX_FAILS" ]; then
        logger -t wifi-watchdog "No connectivity for 15 minutes, reloading WiFi"
        wifi reload_legacy
        echo 0 > "$FAIL_COUNT_FILE"
    fi
fi
