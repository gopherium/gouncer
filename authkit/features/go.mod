module github.com/gopherium/gouncer/authkit/features

go 1.27.3

require (
	github.com/cucumber/godog v0.16.0
	github.com/gopherium/gouncer v0.5.0
	github.com/gopherium/gouncer/authkit v0.0.0
	github.com/gopherium/gouncer/authkit/ratelimit v0.0.0
)

require (
	github.com/cucumber/gherkin/go/v42 v42.0.0 // indirect
	github.com/cucumber/messages/go/v34 v34.2.0 // indirect
	github.com/go-chi/chi/v5 v5.4.0 // indirect
	github.com/go-chi/httprate v0.16.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-memdb v1.3.5 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

replace (
	github.com/gopherium/gouncer/authkit => ../
	github.com/gopherium/gouncer/authkit/ratelimit => ../ratelimit
)
