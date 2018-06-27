package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/rpc"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/dex"
)

func getTokens(client *rpc.Client) ([]dex.Token, error) {
	var tokens dex.TokenState
	err := client.Call("WalletService.Tokens", 0, &tokens)
	if err != nil {
		return nil, err
	}

	return tokens.Tokens, nil
}

func txnPoolSize(client *rpc.Client) (int, error) {
	var size int
	err := client.Call("WalletService.TxnPoolSize", 0, &size)
	if err != nil {
		return 0, err
	}

	return size, nil
}

func nonce(client *rpc.Client, addr consensus.Addr) (uint8, uint64, error) {
	var slot dex.NonceSlot
	err := client.Call("WalletService.Nonce", addr, &slot)
	if err != nil {
		return 0, 0, err
	}

	return slot.Idx, slot.Val, nil
}

func loadCredentials(dir string) ([]consensus.SK, error) {
	var r []consensus.SK
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		if !strings.HasPrefix(f.Name(), "node-") {
			continue
		}

		path := path.Join(dir, f.Name())
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}

		dec := gob.NewDecoder(bytes.NewReader(b))
		var c consensus.NodeCredentials
		err = dec.Decode(&c)
		if err != nil {
			fmt.Printf("error decode credential from file: %s, err: %v, skip\n", path, err)
			continue
		}

		r = append(r, c.SK)
	}

	return r, nil
}

func main() {
	credentialsPath := flag.String("c", "", "path to the directory contains node credentials")
	orderPath := flag.String("order", "", "path to the order file to replay")
	addr := flag.String("addr", ":12001", "node's wallet RPC endpoint")
	flag.Parse()

	client, err := rpc.DialHTTP("tcp", *addr)
	if err != nil {
		panic(err)
	}

	tokens, err := getTokens(client)
	if err != nil {
		panic(err)
	}

	tokenCache := make(map[string]dex.Token)
	for _, t := range tokens {
		tokenCache[strings.ToLower(string(t.Symbol))] = t
	}

	credentials, err := loadCredentials(*credentialsPath)
	if err != nil {
		panic(err)
	}

	f, err := os.Open(*orderPath)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	credIdx := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
	retry:
		poolSize, err := txnPoolSize(client)
		if err != nil {
			panic(err)
		}
		if poolSize > 10000 {
			time.Sleep(10 * time.Millisecond)
			goto retry
		}

		credential := credentials[credIdx]
		if credIdx >= len(credentials) {
			credIdx = 0
		}

		ss := strings.Split(s.Text(), ",")
		market := ss[0]
		ms := strings.Split(market, "_")
		if len(ms) != 2 {
			panic(fmt.Errorf("unknown market format: %s, should be BASE_QUOTE, e.g., ETH_BTC", market))
		}
		base := ms[0]
		baseToken, ok := tokenCache[strings.ToLower(base)]
		if !ok {
			panic(fmt.Errorf("unknown token: %s", base))
		}
		quote := ms[1]
		quoteToken, ok := tokenCache[strings.ToLower(quote)]
		if !ok {
			panic(fmt.Errorf("unknown token: %s", quote))
		}

		var sellSide bool
		side := ss[1]
		if side == "buy" {
			sellSide = false
		} else if side == "sell" {
			sellSide = true
		} else {
			panic(fmt.Errorf("unknown sell position: %s", side))
		}

		price, err := strconv.ParseFloat(ss[2], 64)
		if err != nil {
			panic(err)
		}

		quant, err := strconv.ParseFloat(ss[3], 64)
		if err != nil {
			panic(err)
		}

		priceMul := math.Pow10(int(dex.OrderPriceDecimals))
		priceUnit := uint64(price * priceMul)
		quantMul := math.Pow10(int(quoteToken.Decimals))
		quantUnit := uint64(quant * quantMul)

		nonceIdx, nonceVal, err := nonce(client, credential.MustPK().Addr())
		if err != nil {
			panic(err)
		}

		t := dex.PlaceOrderTxn{
			SellSide:    sellSide,
			Quant:       quantUnit,
			Price:       priceUnit,
			ExpireRound: 0,
			Market:      dex.MarketSymbol{Base: baseToken.ID, Quote: quoteToken.ID},
		}
		txn := dex.MakePlaceOrderTxn(credential, t, nonceIdx, nonceVal)
		err = client.Call("WalletService.SendTxn", txn, nil)
		if err != nil {
			panic(err)
		}
	}

	if s.Err() != nil {
		panic(s.Err())
	}
}
