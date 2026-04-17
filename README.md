# Gabutray

Gabutray adalah aplikasi CLI Go untuk menjalankan profile Xray/V2Ray sebagai
koneksi VPN-style TUN di Linux. Untuk pengguna biasa, Gabutray dipakai dari file
release siap pakai, bukan dari source code.

## Untuk Pengguna

Jangan clone repo kalau hanya ingin memakai aplikasi. Download file binary
release dari halaman **GitHub Releases**:

```text
gabutray-linux-amd64.tar.gz
```

Extract file release:

```bash
tar -xzf gabutray-linux-amd64.tar.gz
cd gabutray-linux-amd64
```

Install dan aktifkan service:

```bash
sudo ./install.sh --enable
```

Mulai dari menu interaktif:

```bash
gabutray menu
```

Di menu, pilih:

- `Tambah Profile` untuk paste link `vless://`, `vmess://`, atau `trojan://`.
- `Pilih & Connect` untuk menghubungkan VPN.
- `Test Profile` untuk cek latency server sebelum connect.
- `Status` untuk melihat koneksi aktif.
- `Disconnect` untuk memutus koneksi.
- `Doctor` untuk cek kesiapan sistem.

Uninstall:

```bash
sudo ./uninstall.sh
```

## Isi Release

Release Linux amd64 berisi:

```text
gabutray-linux-amd64/
  gabutray
  install.sh
  uninstall.sh
  README.md
  engines/
    xray
    tun2socks
    geoip.dat
    geosite.dat
```

Pengguna tidak perlu install Go, compile source code, atau download Xray dan
tun2socks terpisah.

## Command CLI

Menu interaktif:

```bash
gabutray menu
```

Tambah profile lewat command:

```bash
gabutray profile add 'vless://UUID@example.com:443?security=reality&type=tcp&sni=example.com&fp=chrome&pbk=PUBLIC_KEY&sid=SHORT_ID#main'
```

Lihat profile:

```bash
gabutray profile list
```

Test latency semua profile:

```bash
gabutray profile test
```

Test latency satu profile:

```bash
gabutray profile test main --timeout 3s
```

Test ini memakai TCP connect ke server profile, bukan ICMP ping dan bukan
speedtest bandwidth.

Connect:

```bash
gabutray connect main
```

Status:

```bash
gabutray status
```

Disconnect:

```bash
gabutray disconnect
```

## Service

Cetak unit systemd:

```bash
gabutray service print
```

Install dan start service:

```bash
gabutray service install
```

Uninstall service:

```bash
gabutray service uninstall
```

Daemon root memakai socket Unix:

```bash
/run/gabutrayd.sock
```

## Config

Config user default:

```bash
~/.config/gabutray/config.yaml
```

Lihat config:

```bash
gabutray config show
```

Ubah default:

```bash
gabutray config set \
  --xray-bin /opt/gabutray/engines/xray \
  --tun2socks-bin /opt/gabutray/engines/tun2socks \
  --socks-port 10808 \
  --tun-name gabutray0 \
  --tun-cidr 198.18.0.1/15 \
  --dns-enabled=true \
  --dns-strict=true \
  --dns-servers 1.1.1.1,1.0.0.1
```

DNS anti-leak aktif secara default memakai `systemd-resolved` lewat
`resolvectl`. Saat connect, Gabutray mengarahkan DNS ke interface TUN
`gabutray0` dengan domain route `~.`. Saat disconnect, konfigurasi DNS link
tersebut dikembalikan otomatis.

Default DNS server:

```text
1.1.1.1, 1.0.0.1
```

Jika `dns_strict` aktif dan DNS tidak bisa dikonfigurasi, proses connect akan
dibatalkan supaya koneksi tidak berjalan dengan DNS lama.

## Untuk Developer

Developer yang ingin build sendiri perlu Go 1.26 atau lebih baru.

Test dan build lokal:

```bash
go test ./...
go build -o gabutray ./cmd/gabutray
```

Untuk membuat release bundle, pastikan file berikut ada dan executable/data-nya
valid:

```text
third_party/bin/linux-amd64/xray
third_party/bin/linux-amd64/tun2socks
third_party/bin/linux-amd64/geoip.dat
third_party/bin/linux-amd64/geosite.dat
```

Build release Linux amd64:

```bash
scripts/build-linux-amd64.sh
```

Output:

```text
dist/gabutray-linux-amd64.tar.gz
dist/gabutray-linux-amd64.tar.gz.sha256
```

Verify checksum:

```bash
sha256sum -c dist/gabutray-linux-amd64.tar.gz.sha256
```

Release GitHub dibuat otomatis oleh GitHub Actions saat tag versi dipush:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Workflow akan menjalankan test, build bundle, verify checksum, membuat GitHub
Release, dan upload `gabutray-linux-amd64.tar.gz` beserta file `.sha256`.
Jangan arahkan pengguna awam ke source archive otomatis GitHub; arahkan mereka
ke asset `gabutray-linux-amd64.tar.gz`.

## Catatan Implementasi

- Xray dibuat dengan inbound SOCKS lokal di `127.0.0.1:<socks_port>`.
- `tun2socks` dijalankan dengan proxy `socks5://127.0.0.1:<socks_port>`.
- Route default dibagi menjadi `0.0.0.0/1` dan `128.0.0.0/1` menuju interface TUN.
- Route khusus ke IP server dibuat lewat gateway lama supaya koneksi Xray ke server
  tidak berputar masuk TUN.
- DNS anti-leak memakai `resolvectl dns`, `resolvectl domain '~.'`, dan
  `resolvectl default-route` pada interface TUN.
- Profile dan config user disimpan sebagai YAML di `~/.config/gabutray/`.
- Runtime daemon dan log root disimpan di `/run/gabutray/`.

## Batasan v1

- Fokus Linux amd64.
- DNS otomatis saat ini fokus ke sistem yang memakai `systemd-resolved` dan
  menyediakan command `resolvectl`.
- IPv6 route belum diaktifkan.
- Share link advanced yang memakai transport/security di luar `tcp`, `ws`, `grpc`,
  `httpupgrade`, `http/h2`, `none`, `tls`, atau `reality` akan ditolak.
