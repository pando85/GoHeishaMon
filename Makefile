GOCMD=go
GOBUILD=$(GOCMD) build
.POSIX:
.PHONY: *

GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=GoHeishaMon
BINARY_UNIX=$(BINARY_NAME)_AMD64
BINARY_MIPS=$(BINARY_NAME)_MIPS
BINARY_ARM=$(BINARY_NAME)_ARM
BINARY_MIPSUPX=$(BINARY_NAME)_MIPSUPX
KERNEL_IMAGE=openwrt-ar71xx-generic-cus531-16M-kernel.bin
SQUASHFS_IMAGE=openwrt-ar71xx-generic-cus531-16M-rootfs-squashfs.bin

.DEFAULT: help
help:	## show this help menu.
	@echo "Usage: make [TARGET ...]"
	@echo ""
	@@grep -hE "#[#]" $(MAKEFILE_LIST) | sed -e 's/\\$$//' | awk 'BEGIN {FS = "[:=].*?#[#] "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

all:    ## test and build for all platforms
all: test build-linux build-mips build-rpi

build:  ## build binary
build:
	$(GOBUILD) -o $(BINARY_NAME) -v ./src

test:   ## run go tests
test:
	$(GOTEST) -v ./...

clean:  ## clean dist dir
clean:
	$(GOCLEAN)
	rm -f dist/*

run:    ## run main program
run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)

deps:   ## create dist
deps:
	$(GOGET) github.com/eclipse/paho.mqtt.golang
	$(GOGET) github.com/tarm/serial
	$(GOGET) github.com/BurntSushi/toml
	$(GOGET) github.com/rs/xid
	mkdir dist

build-linux:    ## build for linux
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o dist/$(BINARY_UNIX) ./src

build-mips: ## build for MIPS
build-mips:
	CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat $(GOBUILD) -ldflags "-s -w" -a -o dist/$(BINARY_MIPS) ./src

build-rpi:  ## build for ARM
build-rpi:
	GOOS=linux GOARCH=arm GOARM=5 $(GOBUILD) -o dist/$(BINARY_ARM) ./src

upx:    ## package binary
upx:
	@if !upx 2>&1 | grep "UPX 4." > /dev/null; then \
		echo "UPX 4.x is required. Newer version are not supported on current Kernel version;" \
		exit 1; \
	fi
	upx -f --brute -o dist/$(BINARY_MIPSUPX) dist/$(BINARY_MIPS)

compilesquash: ## create root file system
compilesquash:
	cp dist/$(BINARY_MIPSUPX) OS/RootFS/usr/bin/goheishamon
	mksquashfs OS/RootFS dist/$(SQUASHFS_IMAGE) -comp xz -noappend -always-use-fragments -force-uid 0 -force-gid 0

enable-nextboot: ## enable nextboot.sh script
enable-nextboot:
	@cp OS/RootFS/etc/gh/nextboot.sh.disabled OS/RootFS/etc/gh/nextboot.sh

compilesquash-usb: ## create root file system with nextboot.sh script
compilesquash-usb: enable-nextboot compilesquash

install:    ## install in TARGET_HOST.
install: compilesquash
install:
	@if [ ! -f OS/RootFS/etc/dropbear/authorized_keys ]; then \
		echo "Warning: Authorized keys file OS/RootFS/etc/dropbear/authorized_keys not found"; \
		echo "Please copy your public key to this file before updating and change permissions to 600."; \
	fi

	scp -r -O root@$(TARGET_HOST):/etc/config /tmp/config
	echo "Backup written to /tmp/config."
	@if [ ! -f OS/Kernel/$(KERNEL_IMAGE) ]; then \
		echo "Error: Kernel image OS/Kernel/$(KERNEL_IMAGE) not found"; \
		exit 1; \
	fi
	@if [ ! -f dist/$(SQUASHFS_IMAGE) ]; then \
		echo "Error: Squashfs image dist/$(SQUASHFS_IMAGE) not found"; \
		exit 1; \
	fi
	scp -O OS/Kernel/$(KERNEL_IMAGE) root@$(TARGET_HOST):/tmp/
	scp -O dist/$(SQUASHFS_IMAGE) root@$(TARGET_HOST):/tmp/
	ssh root@$(TARGET_HOST) fwupdate fw-write /tmp/$(KERNEL_IMAGE) /tmp/$(SQUASHFS_IMAGE)
	@echo "Now you can reboot your device and enjoy the new version."
	@echo "Remember that your root password has been reset to goheishamon. Please change it."

