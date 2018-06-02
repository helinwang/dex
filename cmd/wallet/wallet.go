package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/dex"
	"github.com/urfave/cli"
)

var rpcAddr string
var credentialPath string

func getTokens(client *rpc.Client) ([]dex.Token, error) {
	var tokens dex.TokenState
	err := client.Call("RPCServer.Tokens", 0, &tokens)
	if err != nil {
		return nil, err
	}

	return tokens.Tokens, nil
}

func parseAddr(accountAddr string) (consensus.Addr, error) {
	var addr consensus.Addr
	b, err := hex.DecodeString(accountAddr)
	if err != nil {
		return addr, err
	}
	copy(addr[:], b)
	return addr, nil
}

func printAccount(c *cli.Context) error {
	var addr consensus.Addr
	accountAddr := c.Args().First()
	if accountAddr == "" {
		c, err := consensus.LoadCredential(credentialPath)
		if err != nil {
			return err
		}

		pk, err := c.SK.PK()
		if err != nil {
			return err
		}

		addr = pk.Addr()
	} else {
		var err error
		addr, err = parseAddr(accountAddr)
		if err != nil {
			return err
		}
	}

	client, err := rpc.DialHTTP("tcp", rpcAddr)
	if err != nil {
		return err
	}

	tokens, err := getTokens(client)
	if err != nil {
		return err
	}

	idToToken := make(map[dex.TokenID]dex.TokenInfo)
	for _, t := range tokens {
		idToToken[t.ID] = t.TokenInfo
	}

	var w dex.WalletState
	err = client.Call("RPCServer.WalletState", addr, &w)
	if err != nil {
		return err
	}

	fmt.Printf("Addr: %x\n", addr[:])
	fmt.Println("Balances:")
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)
	_, err = fmt.Fprintln(tw, "\tSymbol\tAvailable\tPending\t")
	if err != nil {
		return err
	}

	for _, b := range w.Balances {
		symbol := idToToken[b.Token].Symbol
		decimals := int(idToToken[b.Token].Decimals)
		divide := math.Pow10(decimals)
		available := fmt.Sprintf("%.*f", decimals, float64(b.Available)/divide)
		pending := fmt.Sprintf("%.*f", decimals, float64(b.Pending)/divide)
		_, err = fmt.Fprintf(tw, "\t%s\t%s\t%s\t\n", symbol, available, pending)
		if err != nil {
			return err
		}

	}
	err = tw.Flush()
	if err != nil {
		return err
	}

	return nil
}

func sendToken(c *cli.Context) error {
	args := c.Args()
	credential, err := consensus.LoadCredential(credentialPath)
	if err != nil {
		return err
	}

	recipient := args[0]
	symbol := args[1]
	b, err := base64.StdEncoding.DecodeString(recipient)
	if err != nil {
		return fmt.Errorf("PUB_KEY (%s) must be encoded in base64, err: %v", recipient, err)
	}

	pk := consensus.PK(b)
	quant, err := strconv.ParseFloat(args[2], 64)
	if err != nil {
		return err
	}

	client, err := rpc.DialHTTP("tcp", rpcAddr)
	if err != nil {
		return err
	}

	tokens, err := getTokens(client)
	if err != nil {
		return err
	}

	var tokenID dex.TokenID
	var mul float64
	found := false
	for _, t := range tokens {
		if strings.ToLower(string(t.Symbol)) == strings.ToLower(symbol) {
			tokenID = t.ID
			mul = math.Pow10(int(t.Decimals))
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("symbol not found: %s", symbol)
	}

	txn := dex.MakeSendTokenTxn(credential.SK, pk, tokenID, uint64(quant*mul))
	err = client.Call("RPCServer.SendTxn", txn, nil)
	if err != nil {
		return err
	}

	return nil
}

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
		{
			Name:        "send",
			Usage:       "send PUB_KEY SYMBOL AMOUNT (BNB is the native token symbol, PUB_KEY is the recipient's base64 encoded public key)",
			Description: "send native coin or token to recipient",
			Action:      sendToken,
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		fmt.Printf("command failed with error: %v\n", err)
	}
}
