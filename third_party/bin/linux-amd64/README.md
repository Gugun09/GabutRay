# Linux amd64 engines

Place executable release binaries here before building a release bundle:

- `xray`
- `tun2socks`
- `geoip.dat`
- `geosite.dat`

`scripts/build-linux-amd64.sh` refuses to create a release bundle until both
executables and data files exist.
