module github.com/EasterCompany/dex-tts-service

go 1.25.6

require github.com/EasterCompany/dex-go-utils v0.0.0

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/redis/go-redis/v9 v9.17.3 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/text v0.34.0 // indirect
)

replace github.com/EasterCompany/dex-go-utils => ../dex-go-utils
