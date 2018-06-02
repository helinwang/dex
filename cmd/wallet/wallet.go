package main

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math"
	"net/rpc"
	"os"
	"text/tabwriter"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/dex"
	"github.com/urfave/cli"
)

func getTokens(client *rpc.Client) []dex.Token {
	var tokens dex.TokenState
	err := client.Call("RPCServer.Tokens", 0, &tokens)
	if err != nil {
		panic(err)
	}

	return tokens.Tokens
}

func printAccount(c *cli.Context) error {
	var addr consensus.Addr
	accountAddr := c.Args().First()
	if accountAddr == "" {
		b, err := ioutil.ReadFile(credentialPath)
		if err != nil {
			panic(err)
		}

		var c consensus.NodeCredentials
		dec := gob.NewDecoder(bytes.NewReader(b))
		err = dec.Decode(&c)
		if err != nil {
			panic(err)
		}

		pk, err := c.SK.PK()
		if err != nil {
			panic(err)
		}

		addr = pk.Addr()
	} else {
		b, err := hex.DecodeString(accountAddr)
		if err != nil {
			return err
		}
		copy(addr[:], b)
	}

	client, err := rpc.DialHTTP("tcp", rpcAddr)
	if err != nil {
		panic(err)
	}

	tokens := getTokens(client)
	idToToken := make(map[dex.TokenID]dex.TokenInfo)
	for _, t := range tokens {
		idToToken[t.ID] = t.TokenInfo
	}

	var w dex.WalletState
	err = client.Call("RPCServer.WalletState", addr, &w)
	if err != nil {
		panic(err)
	}

	fmt.Printf("addr: %x\n", addr[:])
	fmt.Println("Balances:")
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)
	_, err = fmt.Fprintln(tw, "\tSymbol\tAvailable\tPending\t")
	if err != nil {
		panic(err)
	}

	for _, b := range w.Balances {
		symbol := idToToken[b.Token].Symbol
		decimals := int(idToToken[b.Token].Decimals)
		divide := math.Pow10(decimals)
		available := fmt.Sprintf("%.*f", decimals, float64(b.Available)/divide)
		pending := fmt.Sprintf("%.*f", decimals, float64(b.Pending)/divide)
		_, err = fmt.Fprintf(tw, "\t%s\t%s\t%s\t\n", symbol, available, pending)
		if err != nil {
			panic(err)
		}

	}
	err = tw.Flush()
	if err != nil {
		panic(err)
	}

	return nil
}

var rpcAddr string
var credentialPath string

func main() {
	err := bls.Init(int(bls.CurveFp254BNb))
	if err != nil {
		panic(err)
	}

	app := cli.NewApp()
	app.Name = "DEX wallet"
	app.Usage = ""

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "credential, c",
			Usage:       "path to the node credential file",
			Destination: &credentialPath,
		},
		cli.StringFlag{
			Name:        "addr",
			Value:       ":12001",
			Usage:       "node's wallet RPC endpoint",
			Destination: &rpcAddr,
		},
	}

	app.Commands = []cli.Command{
		{
			Name:   "account",
			Usage:  "print account information",
			Action: printAccount,
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}
