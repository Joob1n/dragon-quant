package fetcher

import (
	"dragon-quant/model"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// 🆕 获取大盘(上证指数) 30分钟K线上下文
func FetchMarket30mKline(days int) string {
	// 000001 (SH Index) -> secid: 1.000001
	// 56 bars = 7 days * 8 bars/day
	limit := days * 8
	url := fmt.Sprintf("http://push2his.eastmoney.com/api/qt/stock/kline/get?secid=1.000001&fields1=f1&fields2=f51,f53,f57,f6&klt=30&fqt=1&end=20500000&lmt=%d", limit)

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("❌ FetchMarketContext Error: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var kResp model.KLineResponse
	json.Unmarshal(body, &kResp)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【上证指数 (000001) - 近%d日 30m走势】:\n", days))

	klines := kResp.Data.Klines
	// Last 56 bars
	count := len(klines)
	if count == 0 {
		return ""
	}

	lastClose := 0.0
	for i, line := range klines {
		// line: date, close, high?, open?, low?, amount, vol?
		// fields2=f51,f53,f57,f6 -> Date, Close, Amount, Volume
		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			p, _ := strconv.ParseFloat(parts[1], 64)
			amt, _ := strconv.ParseFloat(parts[2], 64) // Amount in 元, usually huge

			rate := 0.0
			if i > 0 && lastClose > 0 {
				rate = (p - lastClose) / lastClose * 100
			} else if i == 0 {
				// simple fake for first bar
				lastClose = p
			}

			// Format Amount to 亿
			amtYi := amt / 100000000.0

			// Bar-X: C=3200, R=+0.5%, V=100亿
			sb.WriteString(fmt.Sprintf("[Bar-%d: C=%.0f, R=%.2f%%, V=%.0f亿] ", i+1, p, rate, amtYi))

			lastClose = p
		}
	}
	result := sb.String()
	fmt.Printf("\n🌡️ [Market Context Raw]:\n%s\n", result)
	return result
}
