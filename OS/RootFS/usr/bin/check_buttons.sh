#!/bin/ash

GOHEISHAMON_BIN=/usr/bin/goheishamon
# For the restart below; same reason as in /etc/init.d/goheishamon (golang/go#58212).
export GODEBUG=asyncpreemptoff=1

logger -t check_buttons.sh "Init GPIOs"

# LED
echo 2 > /sys/class/gpio/export
echo 3 > /sys/class/gpio/export
echo 13 > /sys/class/gpio/export
echo 15 > /sys/class/gpio/export

# link
echo 10 > /sys/class/gpio/export

# buttons
echo 0 > /sys/class/gpio/export
echo 1 > /sys/class/gpio/export
echo 16 > /sys/class/gpio/export

while true; do
    # press == `hi`
    ButtonReset=`awk '/gpio-0 /{print $5}' /sys/kernel/debug/gpio`
    # press == `hi`
    ButtonWPS=`awk '/gpio-1 /{print $5}' /sys/kernel/debug/gpio`
    # press == `lo`
    ButtonCheck=`awk '/gpio-16 /{print $5}' /sys/kernel/debug/gpio`
    # Pin for communication by serial port
    CNCNTLink=`awk '/gpio-10 /{print $5}' /sys/kernel/debug/gpio`

    # GoHeishaMon running
    if [ $(ps | grep "$GOHEISHAMON_BIN" | wc -l) -gt 1 ]; then
        # white LED
        echo high > /sys/class/gpio/gpio2/direction
        echo high > /sys/class/gpio/gpio13/direction
        echo high > /sys/class/gpio/gpio15/direction
    else
        # off LED
        echo low > /sys/class/gpio/gpio2/direction
        echo low > /sys/class/gpio/gpio13/direction
        echo low > /sys/class/gpio/gpio15/direction
    fi

    # reset button
    if [ "$ButtonReset" = 'hi' ] && [ "$ButtonWPS" = 'lo' ] && [ "$ButtonCheck" = 'hi' ] ; then
        # yellow LED
        echo low > /sys/class/gpio/gpio2/direction
        echo high > /sys/class/gpio/gpio13/direction
        echo high > /sys/class/gpio/gpio15/direction
        logger -t check_buttons.sh "Restart GoHeishaMon"
        kill $(ps | grep "$GOHEISHAMON_BIN" | head -n1 | awk '{ print $1 }')
        $GOHEISHAMON_BIN | logger -t goheishamon &
    fi

    # WPS button
    if [ "$ButtonReset" = 'lo' ] && [ "$ButtonWPS" = 'hi' ] && [ "$ButtonCheck" = 'hi' ] ; then
        # blue LED
        echo high > /sys/class/gpio/gpio2/direction
        echo low > /sys/class/gpio/gpio13/direction
        echo low > /sys/class/gpio/gpio15/direction
        logger -t check_buttons.sh "Mount USB"
        /usr/bin/usb_mount.sh
        if [ -e /mnt/usb/settings.txt ]; then
            logger -t check_buttons.sh "Connect to SSID from settings.txt"
            /usr/bin/specify_ssid_connect.sh
        fi
        /usr/bin/usb_umount.sh
    fi

    # WPS and reset buttons
    if [ "$ButtonReset" = 'hi' ] && [ "$ButtonWPS" = 'hi' ] && [ "$ButtonCheck" = 'hi' ] ; then
        # green LED
        echo low > /sys/class/gpio/gpio2/direction
        echo high > /sys/class/gpio/gpio13/direction
        echo low > /sys/class/gpio/gpio15/direction
        logger -t check_buttons.sh "Restore root password to default"
        /root/pass.sh goheishamon
    fi

    # all buttons: firmware side switch
    if [ "$ButtonReset" = 'hi' ] && [ "$ButtonWPS" = 'hi' ] && [ "$ButtonCheck" = 'lo' ] ; then
        # red LED
        echo low > /sys/class/gpio/gpio2/direction
        echo low > /sys/class/gpio/gpio13/direction
        echo high > /sys/class/gpio/gpio15/direction
        fwupdate sw > /dev/null 2>&1
        sync
        reboot
    fi

    if [ "$CNCNTLink" = 'hi' ] ; then
        echo low > /sys/class/gpio/gpio3/direction
    fi
    if [ "$CNCNTLink" = 'lo' ] ; then
        echo high > /sys/class/gpio/gpio3/direction
    fi

    sleep 1
done

exit 0
