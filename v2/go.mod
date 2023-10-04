module github.com/nixpare/server/v2

go 1.20

require (
	github.com/Microsoft/go-winio v0.6.1
	github.com/gorilla/securecookie v1.1.1
	github.com/gorilla/websocket v1.5.0
	github.com/nixpare/comms v1.1.0
	github.com/nixpare/logger v1.2.7
	github.com/nixpare/process v1.4.2
	github.com/yookoala/gofast v0.7.0
)

require (
	golang.org/x/mod v0.8.0 // indirect
	golang.org/x/sys v0.5.0 // indirect
	golang.org/x/tools v0.6.0 // indirect
)

retract v2.6.11
