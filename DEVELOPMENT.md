# Development Notes

## Switch firmware

To switch back to the previous partition:

```bash
fwupdate sw
```

## Run code

```bash
make build-mips upx && scp -O dist/GoHeishaMon_MIPSUPX cz-taw1b.iot.grigri:/tmp/goheishamon
```

Then SSH into the device and run:

```bash
/etc/init.d/goheishamon stop
/tmp/goheishamon
```
