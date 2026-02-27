package signal

import (
	"fmt"
	"time"

	"github.com/bullarc/bullarc"
)

const (
	// RiskFlagWhaleExchangeInflow is set when large transfers are heading to exchanges (bearish).
	RiskFlagWhaleExchangeInflow = "whale_exchange_inflow"
	// RiskFlagWhaleColdStorageOutflow is set when large transfers are heading to cold wallets (bullish).
	RiskFlagWhaleColdStorageOutflow = "whale_cold_storage_outflow"
)

// WhaleAlertSignal generates a trading signal from a slice of whale transactions.
// Rules:
//   - Exchange-bound transfers (ToType == "exchange") are bearish → SELL.
//   - Cold-storage-bound transfers (ToType == "wallet") are bullish → BUY.
//   - Mixed or unknown-only transfers → HOLD.
//
// Confidence scales with the total USD volume of the dominant directional flow.
// When txns is empty, nil is returned (no signal).
func WhaleAlertSignal(symbol string, txns []bullarc.WhaleTransaction) *bullarc.Signal {
	if len(txns) == 0 {
		return nil
	}

	var exchangeUSD, walletUSD float64
	for _, tx := range txns {
		switch tx.ToType {
		case "exchange":
			exchangeUSD += tx.AmountUSD
		case "wallet":
			walletUSD += tx.AmountUSD
		}
	}

	totalUSD := exchangeUSD + walletUSD
	if totalUSD == 0 {
		return nil
	}

	var sigType bullarc.SignalType
	var dominant float64
	var riskFlag string

	if exchangeUSD >= walletUSD {
		sigType = bullarc.SignalSell
		dominant = exchangeUSD
		riskFlag = RiskFlagWhaleExchangeInflow
	} else {
		sigType = bullarc.SignalBuy
		dominant = walletUSD
		riskFlag = RiskFlagWhaleColdStorageOutflow
	}

	confidence := (dominant / totalUSD) * 100

	sig := bullarc.Signal{
		Type:        sigType,
		Confidence:  confidence,
		Indicator:   "whale_alert",
		Symbol:      symbol,
		Timestamp:   time.Now().UTC(),
		Explanation: buildWhaleExplanation(sigType, len(txns), totalUSD),
		RiskFlags:   []string{riskFlag},
		Metadata: map[string]any{
			"transaction_count": len(txns),
			"total_usd":         totalUSD,
			"exchange_usd":      exchangeUSD,
			"wallet_usd":        walletUSD,
		},
	}
	return &sig
}

// buildWhaleExplanation returns a human-readable explanation for the whale signal.
func buildWhaleExplanation(sigType bullarc.SignalType, count int, totalUSD float64) string {
	direction := "exchange inflow"
	if sigType == bullarc.SignalBuy {
		direction = "cold storage accumulation"
	}
	return fmt.Sprintf("%d whale transaction(s) totalling $%.0f detected: %s", count, totalUSD, direction)
}
