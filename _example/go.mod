module github.com/go-pkgz/requester/_example

go 1.23

require (
	github.com/go-pkgz/lcw v1.2.0
	github.com/go-pkgz/repeater/v2 v2.2.0
	github.com/go-pkgz/requester v1.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/redis/go-redis/v9 v9.18.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)

replace github.com/go-pkgz/requester => ../
