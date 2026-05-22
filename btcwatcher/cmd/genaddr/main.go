package main

import (
	"fmt"
	"log"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

func main() {
	params := &chaincfg.TestNet3Params

	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		log.Fatalf("generate key: %v", err)
	}

	wif, err := btcutil.NewWIF(privKey, params, true)
	if err != nil {
		log.Fatalf("encode WIF: %v", err)
	}

	hash160 := btcutil.Hash160(privKey.PubKey().SerializeCompressed())
	addr, err := btcutil.NewAddressWitnessPubKeyHash(hash160, params)
	if err != nil {
		log.Fatalf("derive address: %v", err)
	}

	fmt.Printf("Address (testnet3 P2WPKH): %s\n", addr.EncodeAddress())
	fmt.Printf("Private key (WIF):         %s\n", wif.String())
	fmt.Println()
	fmt.Println("Steps:")
	fmt.Println("  1. Send the address to a testnet3 faucet")
	fmt.Println("  2. Note the txid, vout, amount (satoshis), and block height")
	fmt.Println("  3. Set INTEG_ADDR, INTEG_TXID, INTEG_VOUT, INTEG_SATOSHIS, INTEG_START_HEIGHT")
}
