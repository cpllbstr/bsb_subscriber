package main

import (
	token_erc20 "audtion/srv/token"
	"context"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
)

type Transfer struct {
	TxHash common.Hash    `json:"TxHash"`
	Token  common.Address `json:"Token"`
	From   common.Address `json:"From"`
	To     common.Address `json:"To"`
	Value  *big.Int       `json:"Value"`
}

func (t Transfer) String() string {
	return fmt.Sprintf("TxHash: %v Token: %v From: %v To: %v Value: %v", t.TxHash, t.Token, t.From, t.To, t.Value)
}

type AddressHistory struct {
	TouchedTokens map[common.Address]struct{}
	Transfers     []Transfer
}

func NewAddressHistory() AddressHistory {
	return AddressHistory{
		TouchedTokens: make(map[common.Address]struct{}),
		Transfers:     make([]Transfer, 0),
	}
}

func listen(subs *sync.Map, client *ethclient.Client) {
	contractAbi, err := abi.JSON(strings.NewReader(string(token_erc20.Ierc20ABI)))
	if err != nil {
		log.Panic(err)
	}

	query := ethereum.FilterQuery{
		// Addresses: []common.Address{common.HexToAddress("0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984")},
	}

	logTransferSig := []byte("Transfer(address,address,uint256)")
	logTransferSigHash := crypto.Keccak256Hash(logTransferSig)

	logs := make(chan types.Log)
	sub, err := client.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		log.Panic(err)
	}

	for {
		select {
		case err := <-sub.Err():
			log.Panic(err)
		case vLog := <-logs:
			if vLog.Topics[0].Hex() == logTransferSigHash.Hex() {
				var transferEvent Transfer
				if len(vLog.Data) == 0 {
					continue
				}

				transferEvent.Token = common.HexToAddress(vLog.Address.Hex())
				transferEvent.From = common.HexToAddress(vLog.Topics[1].Hex())
				transferEvent.To = common.HexToAddress(vLog.Topics[2].Hex())
				transferEvent.TxHash = vLog.TxHash

				err := contractAbi.UnpackIntoInterface(&transferEvent, "Transfer", vLog.Data)
				if err != nil {
					log.Panic(err)
				}

				fmt.Printf("transferEvent: %v\n", transferEvent)
				val_from, exist_from := subs.Load(transferEvent.From)
				if exist_from {
					hist := val_from.(AddressHistory)
					_, ok := hist.TouchedTokens[transferEvent.Token]
					if !ok {
						hist.TouchedTokens[transferEvent.Token] = struct{}{}
					}

					hist.Transfers = append(hist.Transfers, transferEvent)
					subs.Store(transferEvent.From, hist)
				}

				val_to, exist_to := subs.Load(transferEvent.To)
				if exist_to {
					hist := val_to.(AddressHistory)
					_, ok := hist.TouchedTokens[transferEvent.Token]
					if !ok {
						hist.TouchedTokens[transferEvent.Token] = struct{}{}
					}

					hist.Transfers = append(hist.Transfers, transferEvent)
					subs.Store(transferEvent.From, hist)
				}

			}
		}
	}
}

func getBalance(address, token string, client *ethclient.Client) *big.Int {
	addr, tok := common.HexToAddress(address), common.HexToAddress(token)
	erc20, err := token_erc20.NewIerc20(tok, client)
	if err != nil {
		log.Panic(err)
	}

	balance, err := erc20.BalanceOf(&bind.CallOpts{}, addr)
	if err != nil {
		log.Panic(err)
	}
	return balance
}

func main() {
	var subscribers sync.Map

	client, err := ethclient.Dial("wss://rinkeby.infura.io/ws/v3/a9b9e23f696d4b65970f91b66a8bd46f")
	if err != nil {
		log.Panic(err)
	}

	go listen(&subscribers, client)

	router := gin.Default()

	router.Use(func(Map *sync.Map) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set("subscribers", Map)
			c.Next()
		}
	}(&subscribers))

	router.Use(func(cli *ethclient.Client) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set("client", cli)
			c.Next()
		}
	}(client))

	router.Use(gin.Recovery())

	router.GET("/subscribe", func(c *gin.Context) {
		subs := c.MustGet("subscribers").(*sync.Map)
		address := c.Query("address")
		if address == "" {
			log.Panic("address is not set")
		}
		addr := common.HexToAddress(address)
		_, ok := subs.Load(addr)
		if ok {
			c.String(200, "already exists")
			return
		}
		subs.Store(addr, NewAddressHistory())
		c.String(200, "sucsessfully added")
	})

	router.GET("/subscribers", func(c *gin.Context) {
		subs := c.MustGet("subscribers").(*sync.Map)
		fmt.Printf("subs: %v\n", subs)
		c.JSON(200, subs)
	})

	router.GET("/history", func(ctx *gin.Context) {
		subs := ctx.MustGet("subscribers").(*sync.Map)
		address := ctx.Query("address")
		limit := ctx.Query("limit")
		if address == "" || limit == "" {
			log.Panic("Not all params are set")
		}
		addr := common.HexToAddress(address)
		lim, err := strconv.Atoi(limit)
		fmt.Println(lim)
		if err != nil {
			log.Panic(err)
		}
		val, ok := subs.Load(addr)
		if !ok {
			ctx.JSON(404, fmt.Sprint(addr, " not in map"))
		} else {
			_val := val.(AddressHistory)
			ret_arr := make([]Transfer, 0)
			transferCount := len(_val.Transfers)
			for i := transferCount - 1; i > 0 && i > transferCount-lim; i-- {
				ret_arr = append(ret_arr, _val.Transfers[i])
			}
			ctx.JSON(200, ret_arr)
		}
	})

	router.GET("/balance", func(ctx *gin.Context) {
		address := ctx.Query("address")
		if address == "" {
			log.Panic("address not set")
		}
		token := ctx.Query("token")
		if token == "" {
			log.Panic("token is not set")
		}
		balance := getBalance(address, token, client)
		ctx.JSON(200, balance)
	})

	router.GET("balances", func(ctx *gin.Context) {
		client := ctx.MustGet("client").(*ethclient.Client)
		subs := ctx.MustGet("subscribers").(*sync.Map)

		address := ctx.Query("address")
		if address == "" {
			log.Panic("address not set")
		}
		addr := common.HexToAddress(address)
		history, ok := subs.Load(addr)
		if !ok {
			ctx.JSON(404, fmt.Sprint(addr, " not in map"))
		} else {
			hist := history.(AddressHistory)
			result := make(map[string]*big.Int)
			for token := range hist.TouchedTokens {
				tk := token.String()
				balance := getBalance(address, tk, client)
				fmt.Println(balance)
				result[tk] = balance
			}
			fmt.Printf("result: %v\n", result)
			ctx.JSON(200, result)
		}

	})

	router.Run(":8080")
}
