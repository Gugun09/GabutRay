# Gabutray Release Notes

## Gabutray Release

Target release pertama:

- Linux amd64
- CLI Go
- Menu interaktif untuk pengguna awam
- Dashboard terminal responsif dengan panel status, menu aksi, dan tabel latency
- Menu utama menampilkan profile aktif dan latency realtime setiap 10 detik
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

Release dibuat otomatis oleh GitHub Actions saat tag versi dipush.

Developer cukup jalankan:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

Workflow akan menjalankan test, build bundle, verify checksum, membuat GitHub
Release, lalu upload asset ini:

```text
dist/gabutray-linux-amd64.tar.gz
dist/gabutray-linux-amd64.tar.gz.sha256
```

Isi release notes memakai file `RELEASE.md`, lalu sesuaikan versi dan catatan
perubahan sebelum membuat tag berikutnya.
