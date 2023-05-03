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

For a basic server configuration that serves every connection with the content of the `public` folder located in the working directory,
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


