package main

import (
	"flag"
	"fmt"
	"math/rand"
	"strings"
)

func main() {
	ms := flag.String("markets", "ETH_BTC,BNB_BTC,BNB_ETH", "comma separated market list")
	seed := flag.Int64("seed", 0, "the seed used for the random order generation process")
	count := flag.Int("count", 100000, "order count")
	flag.Parse()
	markets := strings.Split(*ms, ",")

	rand.Seed(*seed)

	lastPrice := 1.0
	lastQuant := 10.0
	for i := 0; i < *count; i++ {
		m := markets[rand.Intn(len(markets))]
		sell := rand.Intn(2) == 0
		var side string
		if sell {
			side = "sell"
		} else {
			side = "buy"
		}
		price := float64(rand.Intn(1001)-500)/10000 + lastPrice
		if price <= 0 {
			price = 1 / 10000
		}
		lastPrice = price

		quant := float64(rand.Intn(1001)-500)/1000 + lastQuant
		if quant <= 0 {
			quant = 1 / 1000
		}
		lastQuant = quant
		fmt.Printf("%s,%s,%f,%f\n", m, side, price, quant)
	}
}
