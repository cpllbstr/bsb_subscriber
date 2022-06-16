package main

import (
	"audtion/srv/token"
	"context"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
)

type Transfer struct {
	TxHash common.Hash
	Token  common.Address
	From   common.Address
	To     common.Address
	Tokens *big.Int
}

func listen(subs *sync.Map) {
	client, err := ethclient.Dial("wss://rinkeby.infura.io/ws/v3/a9b9e23f696d4b65970f91b66a8bd46f")
	if err != nil {
		log.Fatal(err)
	}

	contractAbi, err := abi.JSON(strings.NewReader(string(token.TokenABI)))
	if err != nil {
		log.Fatal(err)
	}

	query := ethereum.FilterQuery{
		Addresses: []common.Address{common.HexToAddress("0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984")},
	}

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
			fmt.Println(vLog)
			if vLog.Topics[0].Hex() == logTransferSigHash.Hex() {
				var transferEvent Transfer
				if len(vLog.Data) == 0 {
					continue
				}
				err := contractAbi.UnpackIntoInterface(&transferEvent, "Transfer", vLog.Data)
				if err != nil {
					log.Fatal(err)
				}

				transferEvent.Token = common.HexToAddress(vLog.Address.Hex())
				transferEvent.From = common.HexToAddress(vLog.Topics[1].Hex())
				transferEvent.To = common.HexToAddress(vLog.Topics[2].Hex())
				transferEvent.TxHash = vLog.TxHash

				val, exist := subs.Load(transferEvent.From)
				if exist {
					val = append(val.([]Transfer), transferEvent)
					subs.Store(transferEvent.From, val)
				}
			}
		}
	}
}

func main() {
	var subscribers sync.Map

	go listen(&subscribers)

	router := gin.Default()

	router.Use(func(Map *sync.Map) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set("subscribers", Map)
			c.Next()
		}
	}(&subscribers))

	router.GET("/subscribe", func(c *gin.Context) {
		subs := c.MustGet("subscribers").(*sync.Map)
		address := c.Query("address")
		if address == "" {
			log.Fatal("address is not set")
		}

		subs.Store(common.HexToAddress(address), make([]Transfer, 0))
	})

	router.GET("/subscribers", func(c *gin.Context) {
		subs := c.MustGet("subscribers").(*sync.Map)
		// keys := make([]string, 0)
		// subs.Range(func(key, value any) bool {
		// 	keys = append(keys, key.(string))
		// 	return true
		// })
		fmt.Printf("subs: %v\n", subs)
		c.JSON(200, subs)
	})

	router.GET("/history", func(ctx *gin.Context) {
		subs := ctx.MustGet("subscribers").(*sync.Map)
		address := ctx.Query("address")
		limit := ctx.Query("limit")
		if address == "" || limit == "" {
			log.Fatal("Not all params are set")
		}
		addr := common.HexToAddress(address)
		lim, err := strconv.Atoi(limit)
		if err != nil {
			log.Fatal(err)
		}
		val, ok := subs.Load(addr)
		if !ok {
			ctx.JSON(404, struct{}{})
		} else {
			_val := val.([]Transfer)
			ctx.JSON(200, _val[len(_val)-lim:])
		}
	})

	router.GET("/balance", func(ctx *gin.Context) {})

	router.Run(":8080")
}
