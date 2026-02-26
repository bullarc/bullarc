package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/bullarc/bullarc"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Continuously analyze a symbol and emit results on new bars",
	RunE:  runWatch,
}

var (
	watchSymbol   string
	watchConfig   string
	watchCSV      string
	watchInterval time.Duration
)

func init() {
	watchCmd.Flags().StringVarP(&watchSymbol, "symbol", "s", "", "symbol to watch (required)")
	_ = watchCmd.MarkFlagRequired("symbol")
	watchCmd.Flags().StringVarP(&watchConfig, "config", "c", "", "path to config file")
	watchCmd.Flags().StringVar(&watchCSV, "csv", "", "path to CSV file for local data")
	watchCmd.Flags().DurationVarP(&watchInterval, "interval", "i", time.Minute, "poll interval")
}

func runWatch(cmd *cobra.Command, _ []string) error {
	e, err := buildEngine(watchConfig, watchCSV)
	if err != nil {
		return err
	}
	fmt.Printf("watching %s every %s (ctrl-c to stop)\n", watchSymbol, watchInterval)
	return e.Watch(cmd.Context(), bullarc.AnalysisRequest{Symbol: watchSymbol}, watchInterval, func(result bullarc.AnalysisResult) {
		printResult(result)
	})
}
