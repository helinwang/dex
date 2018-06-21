package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"os"

	"github.com/helinwang/dex/pkg/consensus"
)

func main() {
	num := flag.Int("N", 100, "number of credentials to generate")
	seed := flag.String("seed", "dex-credentials", "random seed")
	dir := flag.String("dir", "./credentials", "output directory name")
	flag.Parse()

	err := os.MkdirAll(*dir, os.ModePerm)
	if err != nil {
		panic(err)
	}

	rand := consensus.Rand(consensus.SHA3([]byte(*seed)))

	credentials := make([]consensus.NodeCredentials, *num)
	for i := 0; i < *num; i++ {
		credentials[i].SK = rand.SK()
		rand = rand.Derive(rand[:])
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
