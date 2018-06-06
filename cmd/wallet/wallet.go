package main

import (
	"bytes"
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

func nonce(client *rpc.Client, addr consensus.Addr) (uint8, uint64, error) {
	var slot dex.NonceSlot
	err := client.Call("WalletService.Nonce", addr, &slot)
	if err != nil {
		return 0, 0, err
	}

	return slot.Idx, slot.Val, nil
}

func getTokens(client *rpc.Client) ([]dex.Token, error) {
	var tokens dex.TokenState
	err := client.Call("WalletService.Tokens", 0, &tokens)
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
		if len(accountAddr) == len(consensus.ZeroAddr.Hex()) {
			addr, err = parseAddr(accountAddr)
			if err != nil {
				return err
			}
		} else {
			pkStr, err := base64.StdEncoding.DecodeString(accountAddr)
			if err != nil {
				return err
			}

			pk := consensus.PK(pkStr)
			addr = pk.Addr()
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
	err = client.Call("WalletService.WalletState", addr, &w)
	if err != nil {
		return err
	}

	fmt.Printf("Addr:\n%x\n", addr[:])
	fmt.Println("\nBalances:")
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)
	_, err = fmt.Fprintln(tw, "\tSymbol\tAvailable\tPending\t")
	if err != nil {
		return err
	}

	for _, b := range w.Balances {
		symbol := idToToken[b.Token].Symbol
		decimals := int(idToToken[b.Token].Decimals)
		available := quantToStr(b.Available, decimals)
		pending := quantToStr(b.Pending, decimals)
		_, err = fmt.Fprintf(tw, "\t%s\t%s\t%s\t\n", symbol, available, pending)
		if err != nil {
			return err
		}
	}
	err = tw.Flush()
	if err != nil {
		return err
	}

	fmt.Println("\nOrders:")
	tw = tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)
	_, err = fmt.Fprintln(tw, "\tID\tSymbol\tSide\tPrice\tAmount\tExpiry Block Height\t")
	if err != nil {
		return err
	}
	for _, order := range w.Orders {
		side := "buy"
		if order.SellSide {
			side = "sell"
		}

		market := idToToken[order.Market.Base].Symbol + "_" + idToToken[order.Market.Quote].Symbol
		price := quantToStr(order.PriceUnit, dex.OrderPriceDecimals)
		quant := quantToStr(order.QuantUnit, int(idToToken[order.Market.Base].Decimals))
		_, err = fmt.Fprintf(tw, "\t%s\t%s\t%s\t%s\t%s\t%d\t\n", order.ID.Hex(), market, side, price, quant, order.ExpireHeight)
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

func quantToStr(quant uint64, decimals int) string {
	str := strconv.FormatUint(quant, 10)
	if len(str) <= decimals {
		return "0." + string(bytes.Repeat([]byte("0"), decimals-len(str))) + str
	}

	intPart := str[:len(str)-decimals]
	rest := str[len(str)-decimals:]
	return intPart + "." + rest
}

func sendToken(c *cli.Context) error {
	args := c.Args()
	if len(args) < 3 {
		return fmt.Errorf("send needs 3 arguments (received: %d), please check usage using ./wallet -h", len(args))
	}

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

	idx, val, err := nonce(client, credential.SK.MustPK().Addr())
	if err != nil {
		return err
	}

	txn := dex.MakeSendTokenTxn(credential.SK, pk, tokenID, uint64(quant*mul), idx, val)
	err = client.Call("WalletService.SendTxn", txn, nil)
	if err != nil {
		return err
	}

	fmt.Println(consensus.SHA3(txn).Hex())
	return nil
}

func listToken(c *cli.Context) error {
	client, err := rpc.DialHTTP("tcp", rpcAddr)
	if err != nil {
		return err
	}

	tokens, err := getTokens(client)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)
	_, err = fmt.Fprintln(tw, "\tSymbol\tTotal Supply\tDecimals\t")
	if err != nil {
		return err
	}

	for _, t := range tokens {
		decimals := int(t.Decimals)
		supply := quantToStr(t.TotalUnits, decimals)
		_, err = fmt.Fprintf(tw, "\t%s\t%s\t%d\t\n", string(t.Symbol), supply, decimals)
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

func printStatus(c *cli.Context) error {
	client, err := rpc.DialHTTP("tcp", rpcAddr)
	if err != nil {
		return err
	}

	state, err := chainStatus(client)
	if err != nil {
		return err
	}

	var str string
	if state.InSync() {
		str = "In sync"
	} else {
		str = "Out of sync"
	}

	fmt.Printf("%s, round: %d\n", str, state.Round)
	return nil
}

func issueToken(c *cli.Context) error {
	args := c.Args()
	if len(args) < 3 {
		return fmt.Errorf("send needs 3 arguments (received: %d), please check usage using ./wallet -h", len(args))
	}

	symbol := args[0]
	supply, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return err
	}

	decimals, err := strconv.ParseUint(args[2], 10, 8)
	if err != nil {
		return err
	}

	units := supply * uint64(math.Pow10(int(decimals)))

	client, err := rpc.DialHTTP("tcp", rpcAddr)
	if err != nil {
		return err
	}

	tokens, err := getTokens(client)
	if err != nil {
		return err
	}

	for _, t := range tokens {
		// do client side check to provide a better error
		// message (block chain still checks).
		if strings.ToLower(symbol) == strings.ToLower(string(t.Symbol)) {
			return fmt.Errorf("token symbol %s already exists", symbol)
		}
	}

	credential, err := consensus.LoadCredential(credentialPath)
	if err != nil {
		return err
	}

	idx, val, err := nonce(client, credential.SK.MustPK().Addr())
	if err != nil {
		return err
	}

	tokenInfo := dex.TokenInfo{
		Symbol:     dex.TokenSymbol(symbol),
		Decimals:   uint8(decimals),
		TotalUnits: units,
	}

	txn := dex.MakeIssueTokenTxn(credential.SK, tokenInfo, idx, val)
	err = client.Call("WalletService.SendTxn", txn, nil)
	if err != nil {
		return err
	}

	return nil
}

func chainStatus(client *rpc.Client) (consensus.ChainStatus, error) {
	var state consensus.ChainStatus
	err := client.Call("WalletService.ChainStatus", 0, &state)
	if err != nil {
		return state, err
	}

	return state, nil
}

func printGraphviz(c *cli.Context) error {
	client, err := rpc.DialHTTP("tcp", rpcAddr)
	if err != nil {
		return err
	}

	var graph string
	err = client.Call("WalletService.Graphviz", 0, &graph)
	if err != nil {
		return err
	}

	fmt.Println(graph)
	return nil
}

func placeOrder(c *cli.Context) error {
	args := c.Args()
	if len(args) < 5 {
		return fmt.Errorf("send needs 5 arguments (received: %d), please check usage using ./wallet -h", len(args))
	}

	credential, err := consensus.LoadCredential(credentialPath)
	if err != nil {
		return err
	}

	symbol := args[0]
	pair := strings.Split(symbol, "_")
	if len(pair) != 2 {
		return fmt.Errorf("symbol not in correct format, expecting BASE_QUOTE (e.g., ETH_BTC), received: %s", symbol)
	}
	base := strings.ToLower(pair[0])
	quote := strings.ToLower(pair[1])

	side := strings.ToLower(args[1])
	if side != "buy" && side != "sell" {
		return fmt.Errorf("side must be buy or sell, received: %s", side)
	}

	sellSide := side == "sell"
	price, err := strconv.ParseFloat(args[2], 64)
	if err != nil {
		return fmt.Errorf("parse price error: %v", err)
	}

	amount, err := strconv.ParseFloat(args[3], 64)
	if err != nil {
		return fmt.Errorf("parse amount error: %v", err)
	}

	expire, err := strconv.Atoi(args[4])
	if err != nil {
		return fmt.Errorf("parse expiry time error: %v", err)
	}

	client, err := rpc.DialHTTP("tcp", rpcAddr)
	if err != nil {
		return err
	}

	tokens, err := getTokens(client)
	if err != nil {
		return err
	}

	var baseFound, quoteFound bool
	var baseToken, quoteToken dex.Token
	for _, t := range tokens {
		switch strings.ToLower(string(t.Symbol)) {
		case base:
			baseFound = true
			baseToken = t
		case quote:
			quoteFound = true
			quoteToken = t
		}
	}

	if !baseFound {
		return fmt.Errorf("token %s in the market symbol %s is not found in the chain", base, symbol)
	} else if !quoteFound {
		return fmt.Errorf("token %s in the market symbol %s is not found in the chain", quote, symbol)
	}

	market := dex.MarketSymbol{Base: baseToken.ID, Quote: quoteToken.ID}
	quantUnit := uint64(amount * math.Pow10(int(baseToken.Decimals)))
	priceUnit := uint64(price * math.Pow10(int(dex.OrderPriceDecimals)))

	idx, val, err := nonce(client, credential.SK.MustPK().Addr())
	if err != nil {
		return err
	}

	state, err := chainStatus(client)
	if err != nil {
		return err
	}

	expireRound := state.Round + uint64(expire)
	placeOrderTxn := dex.PlaceOrderTxn{
		SellSide:     sellSide,
		QuantUnit:    quantUnit,
		PriceUnit:    priceUnit,
		PlacedHeight: state.Round,
		ExpireHeight: expireRound,
		Market:       market,
	}
	txn := dex.MakePlaceOrderTxn(credential.SK, placeOrderTxn, idx, val)

	err = client.Call("WalletService.SendTxn", txn, nil)
	if err != nil {
		return err
	}

	fmt.Println(consensus.SHA3(txn).Hex())
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
			Name:   "status",
			Usage:  "Print the chain status: ./wallet status",
			Action: printStatus,
		},
		{
			Name:   "graphviz",
			Usage:  "Print the chain visualization in graphviz format, please go to http://www.webgraphviz.com/ for visualization",
			Action: printGraphviz,
		},
		{
			Name:   "token",
			Usage:  "Print the information of every token: ./wallet token",
			Action: listToken,
		},
		{
			Name:   "issue_token",
			Usage:  "Issue new token: ./wallet issue_token SYMBOL TOTAL_SUPPLY DECIMALS",
			Action: issueToken,
		},
		{
			Name:   "send",
			Usage:  "Send native coin or token to recipient's public key: ./wallet send PUB_KEY SYMBOL AMOUNT (BNB is the native token symbol, PUB_KEY is the recipient's base64 encoded public key)",
			Action: sendToken,
		},
		{
			Name:   "account",
			Usage:  "Print account information: ./wallet account PUB_KEY (or ADDRESS), or, ./wallet -c NODE_CREDENTIAL_FILE_PATH account",
			Action: printAccount,
		},
		{
			Name:   "order",
			Usage:  "Place an order: ./wallet -c NODE_CREDENTIAL_FILE_PATH order MARKET_SYMBOL (e.g,. ETH_BTC, ETH is the base asset, BTC is the quote asset) SIDE (buy or sell) PRICE (price=base_asset_value/quote_asset_value) AMOUNT (quantity of base asset) EXPIRY_TIME (in blocks: 0 means won't expire, 1 means expires at the next block, effectively an IOC order)",
			Action: placeOrder,
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		fmt.Printf("command failed with error: %v\n", err)
	}
}
