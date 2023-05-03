# Nix Server (v2)

## Overview
This package provides an HTTP/S server that can be easily
configured for serving static files in few lines of code,
but also scaled to have a server that can handle multiple
domains and subdomains, with their logic for every incoming
request, backgound tasks and event logging.

---

## Package structure
The package mainly relies on the Server object, which listens on
a specific port and forwards every connection to the inner handler.

### Basic Implementation
For a basic `Server` configuration that serves every connection with the content of the `public` folder located in the working directory,
here is how to configure it:
```go
package main

import (
	"os"
	"os/signal"

	"github.com/nixpare/server/v2"
)

func main() {
	// Create a new server on port 8080, not secure
	// (that means using http and not https) and empty
	// path (so the path is the working directory)
	srv, err := server.NewServer(8080, false, "")
	if err != nil {
		panic(err)
	}

	// Register a default route that serves every files inside
	// the /public folder
	srv.RegisterDefaultRoute("Default", server.SubdomainConfig{})
	// The server starts listening on the port
	srv.Start()

	// Listens for a Control-C
	exitC := make(chan os.Signal)
	signal.Notify(exitC, os.Interrupt)
	<- exitC
	// Stops the server after the Control-C
	srv.Stop()
}
```
### Advanced Implementation
For a more anvanced implementation we need a `Router` that will
manage every server listening on different ports and domains,
along with a `TaskManager` that can be used for creating panic-safe
goroutines running in the background.

So we first need a router
```go
// new router with path set to the working directory and log output
// equal to the stdout
router, _ := server.NewRouter("", nil)
```
that will be used to create the servers (we will only implement one)
```go
srv, _ := router.NewServer(443, true, server.Certificate{
	CertPemPath: "path/to/fullchain_key.pem",
	KeyPemPath: "path/to/private_key.pem",
}/*, other certificates concatenated ...*/)
```
then on each server we can register multiple domains and subdmains
```go
mainDomain := srv.RegisterDomain("My Domain", "mydomain.com")
localDomain := srv.RegisterDomain("Localhost Connections", "localhost")

// This registers the subdomain sub.mydomain.com
// serveFunction will be explained below
mainDomain.RegisterSubdomain("sub", server.SubdomainConfig{
	ServeF: serveFunction,
	Website: server.Website{
		Name: "My Website",
		Dir: "Custom Dir holding files",
		PageHeaders: map[string][][2]string{
			"/": {
				{"header_name_1", "value for index page"},
				{"header_name_2", "other value for index page"}
			},
			"/page": {{"header_name", "value for page page"}},
		} // The PageHeaders field can be seen as an object that maps
		  // a string to a series of string couples, each rapresenting
		  // the key-value pair for an http header
	}
})

localDomain.RegisterSubdomain(...)
```

## The serving function - `Route`
To manage every connection with your own logic, the only structures
you need are the `Route` and `Website` ones, and in order to let the
server know which function to call you have to provide one in the
`ServeF` field of the `SubdomainConfig`.
The function must have a signature as descrived by the type
`server.ServeFunction -> func(route *server.Route)`, that is a simple
function taking in input just the Route. This structure holds
everything you need for a web request:
  + functions to serve content (`ServeFile`, `ServeData`, `Error`)
  + functions to manage cookies (`SetCookie`, `DeleteCookie`, `DecodeCookie`)
  + other functions
  + reference to the underlying `http.ResponseWriter` and `*http.Request`
  for using any standard http function from the stardard library
  (or even third party ones that need those type of structures)

## The `TaskManager`
It can used to manage background function running in background,
with panic protection to avoid the crash of the entire program and
the possibility to listen for the router shutdown in order to
interrupt any sensitive procedure. This functions so have a lifecycle
composed of:
 + a startup function that is ran when the task is started for the first time (unless it's later stopped)
 + a cleaup function that is run when the task is stopped
 + an exec function that can be ran manually or by the task timer

The task timer determines if and how ofter the task exec function should be called and can be changed at any time after creation

## Other utility functions
Inside the `utility.go` file there are some useful functions, but
mostly I highlight these two:
 + `PanicToErr`, which can be used to call any function returning an
  error and automatically wrapping them in a panic-safe environment
  that converts the panic into a simple error with a helpful
  stack trace for the panic (or just returns the simple error of
  the function)
  ```go
  func myFunction(arg string) error {
	if arg == "" {
		panic("Empty arg")
	}
	return errors.New(arg)
  }
  myArg = "argument"
  err := PanicToErr(func() error { return myFunction(myArg) })
  // err.Error() is not nil both when the function returned an error
  // or a panic has occurred, but:
  //  - in the first case, only the err.Err field will be set
  //  - in the second case, both the err.PanicErr and err.Stack will be set
  if err.Error() != nil {
	// ...
  }
  ```
  + `RandStr`, which generates a random string with the given length
  populated with the chosen sets of characters (see `CharSet` constants)
