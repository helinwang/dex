package main

import (
	"flag"
	"fmt"
	"math/rand"
	"strings"
)

func main() {
	ms := flag.String("markets", "ETH_BTC,XRP_BTC,XRP_ETH,EOS_BTC,EOS_ETH,ICX_BTC,ICX_ETH,TRX_BTC,TRX_ETH,XLM_BTC,XLM_ETH,BCC_BTC,BCC_ETH,LTC_BTC,LTC_ETH", "comma separated market list")
	seed := flag.Int64("seed", 0, "the seed used for the random order generation process")
	count := flag.Int("count", 100000, "order count")
	flag.Parse()
	markets := strings.Split(*ms, ",")

	rand.Seed(*seed)

	for i := 0; i < *count; i++ {
		m := markets[rand.Intn(len(markets))]
		sell := rand.Intn(2) == 0
		var side string
		if sell {
			side = "sell"
		} else {
			side = "buy"
		}
		price := float64(rand.Intn(5000)) / 10000
		quant := float64(rand.Intn(10)) + 1
		fmt.Printf("%s,%s,%f,%f\n", m, side, price, quant)
	}
}
