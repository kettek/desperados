# DESPerados
This is a program for LAN chatting and user discovery.

## Network Features

  * [x] Multicast UDP Chat
  * [ ] Brute-force IP ranging to discover subnet peers via UDP or TCP.
  * [ ] Direct IP + port messaging via UDP or TCP.
  * [ ] Private + public key cryptography for direct messaging.

## Running

```
go run ./cmd/desp
```

## Commands

  * `/start [ip:port]`
    * Start listening on multicast.
  * `/stop`
    * Stop listening on multicast.
