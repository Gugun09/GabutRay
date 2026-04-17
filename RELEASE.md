# Gabutray Release Notes

## Gabutray v0.1.0

Target release pertama:

- Linux amd64
- CLI Go
- Menu interaktif untuk pengguna awam
- DNS anti-leak otomatis untuk sistem `systemd-resolved`/`resolvectl`
- Test latency TCP profile sebelum connect
- Bundle berisi Gabutray, Xray, tun2socks, `geoip.dat`, dan `geosite.dat`

## Download

Untuk pengguna biasa, download file ini dari GitHub Releases:

```text
gabutray-linux-amd64.tar.gz
```

File `Source code (zip)` dan `Source code (tar.gz)` adalah source code otomatis
dari GitHub. Jangan pakai itu kalau hanya ingin menjalankan aplikasi.

## Install

```bash
tar -xzf gabutray-linux-amd64.tar.gz
cd gabutray-linux-amd64
sudo ./install.sh --enable
gabutray menu
```

## Uninstall

```bash
cd gabutray-linux-amd64
sudo ./uninstall.sh
```

## Verify Checksum

Download juga:

```text
gabutray-linux-amd64.tar.gz.sha256
```

Lalu jalankan:

```bash
sha256sum -c gabutray-linux-amd64.tar.gz.sha256
```

Output yang benar:

```text
gabutray-linux-amd64.tar.gz: OK
```

## Isi Bundle

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

## Cara Upload Release

Developer:

```bash
git tag v0.1.0
git push origin v0.1.0
scripts/build-linux-amd64.sh
```

Upload asset ini ke GitHub Release:

```text
dist/gabutray-linux-amd64.tar.gz
dist/gabutray-linux-amd64.tar.gz.sha256
```

Tempel isi release notes ini di deskripsi GitHub Release, lalu sesuaikan versi
dan catatan perubahan.
