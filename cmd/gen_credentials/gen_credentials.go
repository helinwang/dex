package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"os"

	"github.com/helinwang/dex/pkg/dex"
)

func main() {
	num := flag.Int("N", 1000, "number of credentials to generate")
	dir := flag.String("dir", "./credentials", "output directory name")
	flag.Parse()

	err := os.MkdirAll(*dir, os.ModePerm)
	if err != nil {
		panic(err)
	}

	credentials := make([]dex.Credential, *num)
	for i := 0; i < *num; i++ {
		pk, sk := dex.RandKeyPair()
		credentials[i].SK = sk
		credentials[i].PK = pk
	}

	for i, n := range credentials {
		f, err := os.Create(fmt.Sprintf("%s/node-%d", *dir, i))
		if err != nil {
			panic(err)
		}

		enc := gob.NewEncoder(f)
		err = enc.Encode(n)
		if err != nil {
			panic(err)
		}

		err = f.Close()
		if err != nil {
			panic(err)
		}
	}
}
