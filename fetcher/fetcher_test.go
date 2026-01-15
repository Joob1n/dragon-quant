package fetcher

import (
	"fmt"
	"io/ioutil"
	"testing"
)

// TestFetch1MinKline verifies 1-minute kline fetching
func TestFetch1MinKline(t *testing.T) {
	// Test with a known stock, e.g., Shanghai Index 000001 (using sh000001 or similar logic in fetcher)
	// Fetcher logic: 0.xxxx for SZ, 1.xxxx for SH.
	// Let's use "000001" (PingAn, SZ) or "600519" (Moutai, SH)
	// Or Index "000001" (SH Index) which fetcher handles if passed "000001" with prefix check?
	// In fetcher logic:
	// secID := "0." + code
	// if strings.HasPrefix(code, "6") { secID = "1." + code }
	// So passing "000001" -> "0.000001" (PingAn).

	// code := "000001" // Ping An Bank
	code := "001337"
	days := 7 // Fetch 7 days

	fmt.Printf("Fetching 1-min kline for %s, days=%d...\n", code, days)
	klines := Fetch1MinKline(code, days)

	if len(klines) == 0 {
		t.Errorf("Fetched 0 klines for %s", code)
		return
	}

	fmt.Printf("Success! Fetched %d bars.\n", len(klines))
	if len(klines) > 0 {
		first := klines[0]
		last := klines[len(klines)-1]
		fmt.Printf("First Bar: Time=%s, Close=%.2f\n", first.Date, first.Close)
		fmt.Printf("Last Bar:  Time=%s, Close=%.2f\n", last.Date, last.Close)
	}

	// Basic Validation
	// Basic Validation
	if klines[0].Close <= 0 {
		t.Errorf("Invalid Close price: %.2f", klines[0].Close)
	}

	// 🆕 Save to file for inspection
	filename := "test_1min_kline.txt"
	var sb string
	sb += fmt.Sprintf("Time,Close,Change,Amount\n")
	for _, k := range klines {
		sb += fmt.Sprintf("%s,%.2f,%.2f,%.0f\n", k.Date, k.Close, k.Change, k.Amount)
	}

	err := ioutil.WriteFile(filename, []byte(sb), 0644)
	if err != nil {
		t.Errorf("Failed to write file: %v", err)
	}
	fmt.Printf("Saved %d lines to %s\n", len(klines), filename)

}
