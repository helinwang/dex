package main

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/helinwang/dex/pkg/dex"
)

func main() {
	c := flag.String("c", "", "path to the node credential file")
	flag.Parse()

	b, err := ioutil.ReadFile(*c)
	if err != nil {
		panic(err)
	}

	var credential dex.Credential
	dec := gob.NewDecoder(bytes.NewReader(b))
	err = dec.Decode(&credential)
	if err != nil {
		panic(err)
	}

	fmt.Println("credential info (bytes encoded using base64):")
	skStr := base64.StdEncoding.EncodeToString(credential.SK)
	fmt.Printf("SK: %s\n", skStr)

	pkStr := base64.StdEncoding.EncodeToString(credential.PK)
	fmt.Printf("PK: %s\n", pkStr)

	addr := credential.PK.Addr()
	fmt.Printf("Addr: %x\n", addr[:])
}
