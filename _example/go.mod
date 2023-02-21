module github.com/go-pkgz/requester/_example

go 1.19

require (
	github.com/go-pkgz/lcw v0.8.1
	github.com/go-pkgz/repeater v1.1.3
	github.com/go-pkgz/requester v1.0.0
)

require (
	github.com/go-redis/redis/v7 v7.4.0 // indirect
	github.com/google/uuid v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
)

replace github.com/go-pkgz/requester => ../
