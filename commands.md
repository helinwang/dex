# Commands

The options for all the binaries can be viewed with `./binary_name -h`.

## Node

### Run Nodes

1. Generate the credentials for the trading accounts
    ```
    ./gen_credentials -N 10000  
    ```
    The above command generates 10000 public and secret keys pairs, stored at `./credentials` by default.

1. Generate the genesis file and the initial concensus protocol groups
    - The genesis file contains the genesis block and the genesis state.
    - The initial concensus protocol groups contains the credentials for all the participating nodes and
    the group assignments. The protocol supports open participation (specified but not yet implemented),
    any node can join the mining groups providing proof of frozen fund. Please see the
    [White Paper](https://github.com/helinwang/dex/wiki/White-Paper) for details.
    
    The command below configures 3 nodes and 3 groups with the group threshold set to 2.
    The BNB native token and the tokens specified in `tokens.txt` are distributed evenly
    to all the trading accounts insider the `./credentials` folder.
    
    ```
    ./gen_genesis -N 3 -t 2 -g 3 -tokens tokens.txt -distribute-to ./credentials -dir ./genesis
    ```
    
    Example tokens.txt (can be found in folder `cmd/gen_genesis/`):
    ```
    BTC,90000000000,8
    ETH,90000000000,8
    XRP,90000000000,8
    EOS,90000000000,8
    ICX,90000000000,8
    TRX,90000000000,8
    XLM,90000000000,8
    BCC,90000000000,8
    LTC,90000000000,8
    ```
    Each row is `SYMBOL,SUPPLY,DECIMALS`. BNB is generated as the native token by default, so no need to specify here.

1. Start nodes.
    The total node count is set to 3 and the group threshold is set to 2,
    so running 2 nodes is sufficient for the demonstration purpose.
    1. Start node 0 on port 9000, wallet RPC service is on port 12000
        ```
        ./node -c genesis/nodes/node-0 -genesis genesis/genesis.gob -port 9000 -rpc-addr ":12000"
        ```
    1. Start node 1 on port 9001, wallet RPC service is on port 12001, use `:9000` as the seed node
        ```
        ./node -c genesis/nodes/node-1 -genesis genesis/genesis.gob -port 9001 -rpc-addr ":12001" -seed ":9000"
        ```
    Now you will see the random beacon running, and empty blocks being produced.

## Wallet

The `wallet` binary is a CLI, it talks with the node through the node's wallet RPC service.

### Check Account

```
$ ./wallet -c ./credentials/node-0 account

Addr:
9278552d23bb4cad6e9b1210853f6b9af107f720

Balances:
 |Symbol |Available        |Pending    |Frozen |
 |BNB    |20000.00000000   |0.00000000 |       |
 |BTC    |9000000.00000000 |0.00000000 |       |
 |ETH    |9000000.00000000 |0.00000000 |       |
 |XRP    |9000000.00000000 |0.00000000 |       |
 |EOS    |9000000.00000000 |0.00000000 |       |
 |ICX    |9000000.00000000 |0.00000000 |       |
 |TRX    |9000000.00000000 |0.00000000 |       |
 |XLM    |9000000.00000000 |0.00000000 |       |
 |BCC    |9000000.00000000 |0.00000000 |       |
 |LTC    |9000000.00000000 |0.00000000 |       |

Pending Orders:
 |ID |Market |Side |Price |Amount |Executed |Expiry Block Height |

Execution Reports:
 |Block |ID |Market |Side |Trade Price |Amount |
```

### Trade

Sell 15 ETH at 0.07 BTC, expire after 300 blocks:
```
./wallet -c ./credentials/node-0 order ETH_BTC sell 0.07 15 300
```

Check account:
```
$ ./wallet -c ./credentials/node-0 account   
Addr:
9278552d23bb4cad6e9b1210853f6b9af107f720

Balances:
 |Symbol |Available        |Pending     |Frozen |
 |BNB    |19999.99990000   |0.00000000  |       |
 |BTC    |9000000.00000000 |0.00000000  |       |
 |ETH    |8999985.00000000 |15.00000000 |       |
 |XRP    |9000000.00000000 |0.00000000  |       |
 |EOS    |9000000.00000000 |0.00000000  |       |
 |ICX    |9000000.00000000 |0.00000000  |       |
 |TRX    |9000000.00000000 |0.00000000  |       |
 |XLM    |9000000.00000000 |0.00000000  |       |
 |BCC    |9000000.00000000 |0.00000000  |       |
 |LTC    |9000000.00000000 |0.00000000  |       |

Pending Orders:
 |ID    |Market  |Side |Price      |Amount      |Executed   |Expiry Block Height |
 |2_1_0 |ETH_BTC |SELL |0.07000000 |15.00000000 |0.00000000 |305                 |

Execution Reports:
 |Block |ID |Market |Side |Trade Price |Amount |
```

Buy 10 ETH at 0.08 BTC, expire after 300 blocks:
```
./wallet -c ./credentials/node-0 order ETH_BTC buy 0.08 10 300
```

Check account:
```
$ ./wallet -c ./credentials/node-0 account                         
Addr:
9278552d23bb4cad6e9b1210853f6b9af107f720

Balances:
 |Symbol |Available        |Pending    |Frozen |
 |BNB    |19999.99980000   |0.00000000 |       |
 |BTC    |9000000.00000000 |0.00000000 |       |
 |ETH    |8999995.00000000 |5.00000000 |       |
 |XRP    |9000000.00000000 |0.00000000 |       |
 |EOS    |9000000.00000000 |0.00000000 |       |
 |ICX    |9000000.00000000 |0.00000000 |       |
 |TRX    |9000000.00000000 |0.00000000 |       |
 |XLM    |9000000.00000000 |0.00000000 |       |
 |BCC    |9000000.00000000 |0.00000000 |       |
 |LTC    |9000000.00000000 |0.00000000 |       |

Pending Orders:
 |ID    |Market  |Side |Price      |Amount      |Executed    |Expiry Block Height |
 |2_1_0 |ETH_BTC |SELL |0.07000000 |15.00000000 |10.00000000 |305                 |

Execution Reports:
 |Block |ID    |Market  |Side |Trade Price |Amount      |
 |31    |2_1_1 |ETH_BTC |BUY  |0.07000000  |10.00000000 |
 |31    |2_1_0 |ETH_BTC |SELL |0.07000000  |10.00000000 |
```
