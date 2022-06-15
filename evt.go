package main

import (
	"audtion/srv/token"
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type LogTransfer struct {
	From   common.Address
	To     common.Address
	Tokens *big.Int
}

// LogApproval ..
type LogApproval struct {
	TokenOwner common.Address
	Spender    common.Address
	Tokens     *big.Int
}

func main() {
	client, err := ethclient.Dial("wss://bsc.getblock.io/mainnet/?api_key=7a3454f3-e806-4624-bfd1-60552ce28759")
	if err != nil {
		log.Fatal(err)
	}

	contractAbi, err := abi.JSON(strings.NewReader(string(token.TokenABI)))
	if err != nil {
		log.Fatal(err)
	}

	query := ethereum.FilterQuery{}

	logTransferSig := []byte("Transfer(address,address,uint256)")
	logTransferSigHash := crypto.Keccak256Hash(logTransferSig)

	logs := make(chan types.Log)
	sub, err := client.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		log.Fatal(err)
	}

	for {

		select {
		case err := <-sub.Err():
			log.Fatal(err)
		case vLog := <-logs:
			// fmt.Println(vLog)
			if vLog.Topics[0].Hex() == logTransferSigHash.Hex() {
				fmt.Println("Transfer")

				var transferEvent LogTransfer

				err := contractAbi.UnpackIntoInterface(&transferEvent, "Transfer", vLog.Data)
				if err != nil {
					log.Fatal(err)
				}
				_token := common.HexToAddress(vLog.Address.Hex())
				from := common.HexToAddress(vLog.Topics[1].Hex())
				to := common.HexToAddress(vLog.Topics[2].Hex())

				fmt.Printf("ERC20(%v), %v -> %v %v$\n", _token, from, to, transferEvent.Tokens)

			}
		}
	}
}
