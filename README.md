# SOAP Go Client

## Install

```bash
go get -u github.com/itcomusic/soap
```

## Usage

```go
package main

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"log"

	"github.com/itcomusic/soap"
)

// <urn:ExampleRequest xmlns:urn="something">
//    <Attr>Go</Attr>
// </urn:ExampleRequest>

type Request struct {
	XMLName xml.Name `xml:"urn:ExampleRequest"`
	XmlNS   string   `xml:"xmlns:urn,attr"`
	Attr    string   `xml:"Attr"`
}

func main() {
	c := soap.NewClient("http://127.0.0.1/call", soap.Config{
		BasicAuth: &soap.BasicAuth{
			Username: "test",
			Password: "test",
		},
		TLS: &tls.Config{InsecureSkipVerify: true}})
	
    ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
    defer cancel()
	if err := c.Call(ctx, "", Request{
		XmlNS: "something",
		Attr:  "Go",
	}, nil); err != nil {
		log.Fatal(err)
	}
}

```