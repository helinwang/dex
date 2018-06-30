# Commands

Let's go through the commands by examples. The options for all the binaries can be viewed with `./binary_name -h`.

## Node

### Run Nodes

1. Generate the credentials for the trading accounts
    ```
    $ ./gen_credentials -N 10000  
    ```
    The above command generates 10000 public and secret keys pairs, stored at `./credentials` by default.

1. Generate the genesis file and the initial consensus protocol group files
    - The genesis file contains the genesis block and the genesis state.
    - The initial consensus protocol group files contain the credentials for all the participating nodes and
    the group assignments. The protocol supports open participation (specified but not yet implemented),
    any node can join the mining groups providing proof of frozen fund. Please see the
    [White Paper](https://github.com/helinwang/dex/wiki/White-Paper) for details.
    
    The command below configures three nodes and three groups with the group threshold set to two.
    The BNB native token and the tokens specified in `tokens.txt` are distributed evenly
    to all the trading accounts insider the `./credentials` folder.
    
    ```
    $ ./gen_genesis -N 3 -t 2 -g 3 -tokens tokens.txt -distribute-to ./credentials -dir ./genesis
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
    The total node count is three, and the group threshold is two,
    so running two nodes is sufficient for the demonstration purpose.
    1. Start node 0 on port 9000, wallet RPC service is on port 12000
        ```
        $ ./node -c genesis/nodes/node-0 -genesis genesis/genesis.gob -port 9000 -rpc-addr ":12000"
        ```
    1. Start node 1 on port 9001, wallet RPC service is on port 12001, use `:9000` as the seed node
        ```
        $ ./node -c genesis/nodes/node-1 -genesis genesis/genesis.gob -port 9001 -rpc-addr ":12001" -seed ":9000"
        ```
    Now you will see the random beacon running, and empty blocks being produced.

## Wallet

The `wallet` binary is a CLI. It talks with the node through the node's wallet RPC service.

### Trade

Sell 15 ETH at 0.07 BTC, expire after 300 blocks:
```
$ ./wallet -c ./credentials/node-0 order ETH_BTC sell 0.07 15 300
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
$ ./wallet -c ./credentials/node-0 order ETH_BTC buy 0.08 10 300
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

You can see the orders were matched according to time priority, execution reports are generated for each execution,
and the pending order is shown as well. Also, a flat 0.0001 BNB fee is charged per transaction.
I did not have enough time to implement the percentage-based trading fee, or adjustable fee according to the network condition.
But it would not be too hard to implement.

Cancel Order:
```
$ ./wallet -c ./credentials/node-0 cancel 2_1_0
```
Please note that cancelling an order will not generate execution report.

### Issue Token

Issue HELIN_COIN, total supply 999999, decimals 8:
```
$ ./wallet -c ./credentials/node-0 issue_token HELIN_COIN 999999 8
```

### List All Tokens

```
$ ./wallet token
 |     Symbol|         Total Supply| Decimals|
 |        BNB|   200000000.00000000|        8|
 |        BTC| 90000000000.00000000|        8|
 |        ETH| 90000000000.00000000|        8|
 |        XRP| 90000000000.00000000|        8|
 |        EOS| 90000000000.00000000|        8|
 |        ICX| 90000000000.00000000|        8|
 |        TRX| 90000000000.00000000|        8|
 |        XLM| 90000000000.00000000|        8|
 |        BCC| 90000000000.00000000|        8|
 |        LTC| 90000000000.00000000|        8|
 | HELIN_COIN|      999999.00000000|        8|
```

### Send Token

Due to time constraint, I only implemented send to public key, send to address is easy to add.

1. Get the public key of the account 1
    ```
    $ ./credential_info -c credentials/node-1
    credential info (bytes encoded using base64):
    SK: hDTgUQxmwGCaG/abozy/iIMHiT1S3OtlxFAa5TRmmRU=
    PK: BAv9dVwsREUF5dn1iIiGAioDB7bvE/fiXopXiFkj58eO7VlXzF9srrnNy1d4c7Kcqm8Niv4yeBQKRlwQLnUFDBQ=
    Addr: c09676fdec88c1e960e6398f1c281defdd1cb4fa
    ```
1. Send to account 1's public key:
    ```
    $ ./wallet -c ./credentials/node-0 send BAv9dVwsREUF5dn1iIiGAioDB7bvE/fiXopXiFkj58eO7VlXzF9srrnNy1d4c7Kcqm8Niv4yeBQKRlwQLnUFDBQ= HELIN_COIN 20
    ```
    
    Verify account 1 received it:
    ```
    $ ./wallet -c ./credentials/node-1 account
    Addr:
    c09676fdec88c1e960e6398f1c281defdd1cb4fa

    Balances:
     |Symbol     |Available        |Pending    |Frozen |
     |BNB        |20000.00000000   |0.00000000 |       |
     |BTC        |9000000.00000000 |0.00000000 |       |
     |ETH        |9000000.00000000 |0.00000000 |       |
     |XRP        |9000000.00000000 |0.00000000 |       |
     |EOS        |9000000.00000000 |0.00000000 |       |
     |ICX        |9000000.00000000 |0.00000000 |       |
     |TRX        |9000000.00000000 |0.00000000 |       |
     |XLM        |9000000.00000000 |0.00000000 |       |
     |BCC        |9000000.00000000 |0.00000000 |       |
     |LTC        |9000000.00000000 |0.00000000 |       |
     |HELIN_COIN |20.00000000      |0.00000000 |       |
    
    Pending Orders:
     |ID |Market |Side |Price |Amount |Executed |Expiry Block Height |

    Execution Reports:
     |Block |ID |Market |Side |Trade Price |Amount |
    ```

### Freeze Token

Freeze 10000 BNB at round (round is same as block height) 500
```
$ ./wallet -c ./credentials/node-0 freeze BNB 10000 500

$ ./wallet -c ./credentials/node-0 account             
Addr:
9278552d23bb4cad6e9b1210853f6b9af107f720

Balances:
 |Symbol |Available        |Pending    |Frozen             |
 |BNB    |9999.99990000    |0.00000000 |10000.00000000@500 |
 |BTC    |9000000.00000000 |0.00000000 |                   |
 |ETH    |9000000.00000000 |0.00000000 |                   |
 |XRP    |9000000.00000000 |0.00000000 |                   |
 |EOS    |9000000.00000000 |0.00000000 |                   |
 |ICX    |9000000.00000000 |0.00000000 |                   |
 |TRX    |9000000.00000000 |0.00000000 |                   |
 |XLM    |9000000.00000000 |0.00000000 |                   |
 |BCC    |9000000.00000000 |0.00000000 |                   |
 |LTC    |9000000.00000000 |0.00000000 |                   |

Pending Orders:
 |ID |Market |Side |Price |Amount |Executed |Expiry Block Height |

Execution Reports:
 |Block |ID |Market |Side |Trade Price |Amount |
```

Please note that after implementing the freeze function, I realized the freeze function in BNB's Ether contract is freeze until unfrozen, rather than freeze until block height.
I did not have a chance to match this behavior, but it would be easy to implement.

### Burn Token

Burn 1000 BTC:
```
$ ./wallet -c ./credentials/node-0 burn BTC 1000
```
The total supply of BTC is reduced as well:
```
$ ./wallet token  
 | Symbol|         Total Supply| Decimals|
 |    BNB|   200000000.00000000|        8|
 |    BTC| 89999999000.00000000|        8|
 |    ETH| 90000000000.00000000|        8|
 |    XRP| 90000000000.00000000|        8|
 |    EOS| 90000000000.00000000|        8|
 |    ICX| 90000000000.00000000|        8|
 |    TRX| 90000000000.00000000|        8|
 |    XLM| 90000000000.00000000|        8|
 |    BCC| 90000000000.00000000|        8|
 |    LTC| 90000000000.00000000|        8|
```

### Check Chain Status

```
$ ./wallet status
In sync, round: 128
Metrics of last 10 rounds:
 | Round|   Block Time| Transaction Count|
 |   127| 1.008702519s|                 0|
 |   126| 1.008460589s|                 0|
 |   125| 1.011787425s|                 0|
 |   124| 1.006500142s|                 0|
 |   123|  1.01291797s|                 0|
 |   122| 1.007379805s|                 0|
 |   121| 1.011837359s|                 0|
 |   120| 1.006966881s|                 0|
 |   119|  1.01126981s|                 0|
 |   118| 1.008079414s|                 0|
Stats
 | Number of Rounds| Average Block Time| Transaction per Second|
 |                3|       1.009650177s|               0.000000|
 |               10|       1.009390191s|               0.000000|
 |               30|       1.009942222s|               0.000000|
 |              100|       1.009953654s|               0.019803|
```

### Draw Chain's Blocks

```
$ ./wallet graphviz                        
digraph chain {
rankdir=LR;
size="12,8"
node [shape = rect, style=filled, color = chartreuse2]; block_c669 block_2616 block_6595 num_blocks_omitted_to_save_space_148 block_aebe block_a4e1 block_bca4
node [shape = rect, style=filled, color = aquamarine]; block_2d04 block_54e3
block_c669 -> block_2616 -> block_6595 -> num_blocks_omitted_to_save_space_148 -> block_aebe -> block_a4e1 -> block_bca4
block_bca4 -> block_2d04
block_2d04 -> block_54e3

}
```

It prints the blockchain representation in the graphviz format.
You can paste it to http://www.webgraphviz.com/ to see the visualization.
The green block is the finalized block. The blue block is the non-finalized block.
