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

