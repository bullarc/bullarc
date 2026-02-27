package signal

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/bullarc/bullarc"
)

// OnChainSignal generates a trading signal by combining exchange net flow data
// (ChainMetrics) with whale transaction patterns.
//
// Rules:
//   - Non-crypto symbols (no "/" in symbol) produce no signal.
//   - BUY when chain shows net outflow AND whales are accumulating (moving to cold wallets).
//   - SELL when chain shows net inflow AND whales are distributing (moving to exchanges).
//   - HOLD when the two data sources disagree.
//   - nil is returned when the symbol is non-crypto or data is insufficient.
//
// Confidence scales with the directional dominance of whale transactions by USD value
// and is further modulated by the magnitude of the on-chain net flow.
func OnChainSignal(symbol string, metrics *bullarc.ChainMetrics, txns []bullarc.WhaleTransaction) *bullarc.Signal {
	if !strings.Contains(symbol, "/") {
		return nil
	}
	if metrics == nil || len(txns) == 0 {
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

	chainOutflow := metrics.FlowDirection == bullarc.FlowDirectionOutflow
	chainInflow := metrics.FlowDirection == bullarc.FlowDirectionInflow

	var sigType bullarc.SignalType
	var dominant float64

	switch {
	case chainOutflow && walletUSD > exchangeUSD:
		sigType = bullarc.SignalBuy
		dominant = walletUSD
	case chainInflow && exchangeUSD > walletUSD:
		sigType = bullarc.SignalSell
		dominant = exchangeUSD
	default:
		sigType = bullarc.SignalHold
		dominant = math.Max(exchangeUSD, walletUSD)
	}

	var confidence float64
	if sigType == bullarc.SignalHold {
		confidence = 50.0
	} else {
		// Base confidence from whale directional dominance (50–100 range).
		whaleConf := (dominant / totalUSD) * 100

		// Boost proportional to absolute net flow magnitude (each 1000 units adds up to 5%).
		// This satisfies "proportional to magnitude of net flow".
		flowBoost := math.Min(math.Abs(metrics.NetFlow)/1000*5, 10)

		confidence = math.Min(whaleConf+flowBoost, 100)
	}

	ts := metrics.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	return &bullarc.Signal{
		Type:        sigType,
		Confidence:  confidence,
		Indicator:   "on_chain",
		Symbol:      symbol,
		Timestamp:   ts,
		Explanation: buildOnChainExplanation(sigType, len(txns), totalUSD, metrics.NetFlow),
		Metadata: map[string]any{
			"net_flow":          metrics.NetFlow,
			"flow_direction":    string(metrics.FlowDirection),
			"transaction_count": len(txns),
			"total_usd":         totalUSD,
			"exchange_usd":      exchangeUSD,
			"wallet_usd":        walletUSD,
		},
	}
}

func buildOnChainExplanation(sigType bullarc.SignalType, txnCount int, totalUSD, netFlow float64) string {
	direction := "mixed on-chain signals"
	switch sigType {
	case bullarc.SignalBuy:
		direction = "net outflow + whale accumulation"
	case bullarc.SignalSell:
		direction = "net inflow + whale distribution"
	}
	return fmt.Sprintf(
		"%d whale transaction(s) totalling $%.0f, net flow %.2f: %s",
		txnCount, totalUSD, netFlow, direction,
	)
}
