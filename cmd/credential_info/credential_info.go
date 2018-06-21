package main

import (
	"encoding/base64"
	"flag"
	"fmt"

	"github.com/helinwang/dex/pkg/consensus"
)

func main() {
	c := flag.String("c", "", "path to the node credential file")
	flag.Parse()

	credential, err := consensus.LoadCredential(*c)
	if err != nil {
		panic(err)
	}

	fmt.Println("credential info (bytes encoded using base64):")
	skStr := base64.StdEncoding.EncodeToString(credential.SK)
	fmt.Printf("SK: %s\n", skStr)

	pk, err := credential.SK.PK()
	if err != nil {
		panic(err)
	}

	pkStr := base64.StdEncoding.EncodeToString(pk)
	fmt.Printf("PK: %s\n", pkStr)

	addr := pk.Addr()
	fmt.Printf("Addr: %x\n", addr[:])
}
