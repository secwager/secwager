package internal

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

// outputAddress extracts the single pay-to address from a pkScript, if any.
func outputAddress(pkScript []byte, params *chaincfg.Params) (string, error) {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, params)
	if err != nil {
		return "", err
	}
	if len(addrs) != 1 {
		return "", fmt.Errorf("expected 1 address, got %d", len(addrs))
	}
	return addrs[0].EncodeAddress(), nil
}
