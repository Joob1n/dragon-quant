package fetcher

import (
	"dragon-quant/model"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func FetchSectorStocks(code string) []model.StockInfo {
	cleanCode := strings.ReplaceAll(code, "BK", "")
	// 🔥 f19:竞价金额, f62:净流入, f7:振幅
	url := fmt.Sprintf("http://push2.eastmoney.com/api/qt/clist/get?pn=1&pz=500&po=1&np=1&fltt=2&invt=2&fid=f3&fs=b:BK%s&fields=f12,f14,f2,f3,f8,f10,f62,f7,f19,f267,f164", cleanCode)
	items := FetchRaw(url)
	var list []model.StockInfo
	for _, item := range items {
		var s model.StockInfo
		json.Unmarshal(item, &s)
		list = append(list, s)
	}
	return list
}

// 🆕 FetchSectorHistory fetches the daily K-line history for a sector index.
func FetchSectorHistory(code string) []model.KLineData {
	// EastMoney Block ID format: "BK0xxx" -> "90.BK0xxx"
	// For industry like "BK0477", use "90.BK0477"
	// For concept like "BK0984", use "90.BK0984"
	// Note: Usually "90." works for blocks.
	secID := "90." + code

	// klt=101: Daily
	// lmt=15: Get last 15 days (enough for trend analysis)
	// Change f6 -> f57 (Amount)
	url := fmt.Sprintf("http://push2his.eastmoney.com/api/qt/stock/kline/get?secid=%s&fields1=f1&fields2=f51,f53,f57&klt=101&fqt=1&end=20500000&lmt=15", secID)

	// fmt.Printf("DEBUG: FetchSectorHistory URL: %s\n", url)

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("❌ FetchSectorHistory Net Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	// fmt.Printf("DEBUG: FetchSectorHistory Body: %s\n", string(body)[:min(len(body), 200)])

	var kResp model.KLineResponse
	json.Unmarshal(body, &kResp)

	var klines []model.KLineData
	lastClose := 0.0
	for i, line := range kResp.Data.Klines {
		// fmt.Printf("DEBUG Line: %s\n", line)
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			p, _ := strconv.ParseFloat(parts[1], 64)
			amt := 0.0
			if len(parts) >= 3 {
				amt, _ = strconv.ParseFloat(parts[2], 64)
			}
			change := 0.0
			if i > 0 {
				change = (p - lastClose) / lastClose * 100 // Convert to PctChange for easier AI reading
			}
			lastClose = p
			klines = append(klines, model.KLineData{Close: p, Change: change, Amount: amt})
		}
	}
	return klines
}

func FetchHistoryData(code string, limit int) []model.KLineData {
	secID := "0." + code
	if strings.HasPrefix(code, "6") {
		secID = "1." + code
	}
	// klt=101: 日线
	// fields2=f51,f53,f6 (Date, Close, Amount)
	url := fmt.Sprintf("http://push2his.eastmoney.com/api/qt/stock/kline/get?secid=%s&fields1=f1&fields2=f51,f53,f6&klt=101&fqt=1&end=20500000&lmt=%d", secID, limit)

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var kResp model.KLineResponse
	json.Unmarshal(body, &kResp)

	var klines []model.KLineData
	lastClose := 0.0
	for i, line := range kResp.Data.Klines {
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			p, _ := strconv.ParseFloat(parts[1], 64)
			amt := 0.0
			if len(parts) >= 3 {
				amt, _ = strconv.ParseFloat(parts[2], 64)
			}
			change := 0.0
			if i > 0 {
				change = p - lastClose
			}
			lastClose = p
			klines = append(klines, model.KLineData{Close: p, Change: change, Amount: amt, Date: parts[0]})
		}
	}
	return klines
}

// 🆕 获取市场情绪 (昨日涨停表现)
func FetchSentimentIndex() float64 {
	// BK0815: 昨日涨停
	url := "http://push2.eastmoney.com/api/qt/clist/get?pn=1&pz=500&po=1&np=1&fltt=2&invt=2&fid=f3&fs=b:BK0815&fields=f3"
	items := FetchRaw(url)
	totalChange := 0.0
	count := 0
	for _, item := range items {
		var s struct {
			Change float64 `json:"f3"`
		}
		if err := json.Unmarshal(item, &s); err == nil {
			totalChange += s.Change
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return totalChange / float64(count)
}

// 🆕 获取5分钟K线数据 (用于计算开盘承接率)
func Fetch5MinKline(code string) []model.KLineData {
	secID := "0." + code
	if strings.HasPrefix(code, "6") {
		secID = "1." + code
	}
	// klt=5: 5分钟
	// fields2=f51,f57 (Date, Amount)
	url := fmt.Sprintf("http://push2his.eastmoney.com/api/qt/stock/kline/get?secid=%s&fields1=f1&fields2=f51,f57&klt=5&fqt=1&end=20500000&lmt=10", secID)

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var kResp model.KLineResponse
	json.Unmarshal(body, &kResp)

	var klines []model.KLineData
	for _, line := range kResp.Data.Klines {
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			// fields2=f51,f57 -> part[0]=Date, part[1]=Amount
			amt, _ := strconv.ParseFloat(parts[1], 64)
			klines = append(klines, model.KLineData{Close: 0, Change: 0, Amount: amt})
		}
	}
	return klines
}

// 🆕 获取30分钟K线数据
func Fetch30MinKline(code string, limit int) []model.KLineData {
	secID := "0." + code
	if strings.HasPrefix(code, "6") {
		secID = "1." + code
	}
	// klt=30: 30分钟
	// fields2=f51,f53,f57 (Date, Close, Amount)
	url := fmt.Sprintf("http://push2his.eastmoney.com/api/qt/stock/kline/get?secid=%s&fields1=f1&fields2=f51,f53,f57&klt=30&fqt=1&end=20500000&lmt=%d", secID, limit)

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var kResp model.KLineResponse
	json.Unmarshal(body, &kResp)

	var klines []model.KLineData
	lastClose := 0.0
	for i, line := range kResp.Data.Klines {
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			p, _ := strconv.ParseFloat(parts[1], 64)
			amt := 0.0
			if len(parts) >= 3 {
				amt, _ = strconv.ParseFloat(parts[2], 64)
			}
			change := 0.0
			if i > 0 {
				change = p - lastClose
			}
			lastClose = p
			klines = append(klines, model.KLineData{Close: p, Change: change, Amount: amt})
		}
	}
	return klines
}

// 🆕 获取1分钟K线数据 (指定天数)
func Fetch1MinKline(code string, days int) []model.KLineData {
	// 1. Get Trading Days (Daily K-line)
	// We need 'days' trading days.
	dailyKlines := FetchHistoryData(code, days)
	if len(dailyKlines) == 0 {
		return nil
	}

	secID := "0." + code
	if strings.HasPrefix(code, "6") {
		secID = "1." + code
	}

	var allMinKlines []model.KLineData
	client := http.Client{Timeout: 10 * time.Second}

	// 2. Loop over each day to get 1-min data
	for _, day := range dailyKlines {
		// day.Date format is usually "2006-01-02"
		dateStr := strings.ReplaceAll(day.Date, "-", "") // "20060102"

		// lmt=240 for one day
		url := fmt.Sprintf("http://push2his.eastmoney.com/api/qt/stock/kline/get?secid=%s&fields1=f1&fields2=f51,f53,f57&klt=1&fqt=1&end=%s&lmt=240", secID, dateStr)

		resp, err := client.Get(url)
		if err != nil {
			fmt.Printf("Error 1m: %v\n", err)
			continue
		}

		body, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		var kResp model.KLineResponse
		json.Unmarshal(body, &kResp)

		lastClose := 0.0 // Actually we might not need to recalculate change carefully if we just want data
		// But let's keep consistency

		for i, line := range kResp.Data.Klines {
			parts := strings.Split(line, ",")
			if len(parts) >= 2 {
				p, _ := strconv.ParseFloat(parts[1], 64)
				amt := 0.0
				if len(parts) >= 3 {
					amt, _ = strconv.ParseFloat(parts[2], 64)
				}
				change := 0.0
				// For change, we really need prev close of *fetching session*?
				// Simplification: just diff with previous bar in this chunk
				if i > 0 {
					change = p - lastClose
				}
				lastClose = p

				// Optional: Filter to ensure we only keep bars for THIS day?
				// Usually lmt=240 + end=Date gives that day's data.
				// Just append.
				allMinKlines = append(allMinKlines, model.KLineData{Close: p, Change: change, Amount: amt, Date: parts[0]})
			}
		}
	}
	// Note: They might be in chronological order if dailyKlines is chronological.
	return allMinKlines
}

func FetchTopSectors(fs string, limit int, typeName string) []model.SectorInfo {
	// Add f62 (NetInflow), f164 (5-Day NetInflow)
	url := fmt.Sprintf("http://push2.eastmoney.com/api/qt/clist/get?pn=1&pz=%d&po=1&np=1&fltt=2&invt=2&fid=f3&fs=%s&fields=f12,f14,f62,f164", limit, fs)
	items := FetchRaw(url)
	var list []model.SectorInfo
	for _, item := range items {
		var s model.SectorInfo
		json.Unmarshal(item, &s)
		s.Type = typeName
		list = append(list, s)
	}
	return list
}

func FetchRaw(url string) []json.RawMessage {
	// Debug: Print URL and Response
	// fmt.Printf("Fetching: %s\n", url)
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("❌ FetchRaw Error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	// fmt.Printf("Raw Body (Top 100): %s\n", string(body)[:min(len(body), 100)])

	var wrap model.ListResponse
	json.Unmarshal(body, &wrap)
	return wrap.Data.Diff
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 🆕 获取个股详情 (竞价 f277 + 盘口)
func FetchStockDetails(s *model.StockInfo) {
	secID := "0." + s.Code
	if strings.HasPrefix(s.Code, "6") {
		secID = "1." + s.Code
	}
	// f277: 竞价金额/开盘金额
	// f19: 买一价, f20: 买一量, f17: 卖一价, f18: 卖一量 (注意：这里用的是详细接口，f19定义可能与列表接口不同，但Debug中f277是关键)
	url := fmt.Sprintf("http://push2.eastmoney.com/api/qt/stock/get?secid=%s&fields=f19,f20,f17,f18,f277", secID)

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	// Quick dirty parse because struct is complex
	// Response: {"data":{"f19":..., "f277":...}}
	var wrapper struct {
		Data struct {
			Buy1Price float64 `json:"f19"`
			Buy1Vol   int     `json:"f20"`
			Sell1Vol  int     `json:"f18"`
			CallAmt   float64 `json:"f277"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil {
		s.CallAuctionAmt = wrapper.Data.CallAmt
		s.Buy1Price = wrapper.Data.Buy1Price
		s.Buy1Vol = wrapper.Data.Buy1Vol
		s.Sell1Vol = wrapper.Data.Sell1Vol
	}
}

// 🆕 获取龙虎榜数据
func FetchLHBData(s *model.StockInfo) {
	// 尝试获取最新一期的龙虎榜
	// 逻辑：尝试今天，如果今天是周末或未出榜，可能拿不到，这里简单尝试最近日期
	// 实际工程中应该遍历最近几日。这里为了演示，硬编码尝试 "2026-01-09" (根据Debug结果) 以及 Today
	dates := []string{time.Now().Format("2006-01-02"), "2026-01-09"}

	for _, d := range dates {
		url := fmt.Sprintf("https://datacenter-web.eastmoney.com/api/data/v1/get?reportName=RPT_DAILYBILLBOARD_DETAILS&columns=ALL&filter=(SECURITY_CODE%%3D%%22%s%%22)(TRADE_DATE%%3D%%27%s%%27)", s.Code, d)

		client := http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		body, _ := ioutil.ReadAll(resp.Body)

		if strings.Contains(string(body), "\"result\":null") {
			continue
		}

		// 解析
		var lhbResp struct {
			Result struct {
				Data []struct {
					Explain string  `json:"EXPLAIN"`           // "买一主买"
					NetAmt  float64 `json:"BILLBOARD_NET_AMT"` // 净买入
					BuyAmt  float64 `json:"BILLBOARD_BUY_AMT"`
					SellAmt float64 `json:"BILLBOARD_SELL_AMT"`
				} `json:"data"`
			} `json:"result"`
		}

		if err := json.Unmarshal(body, &lhbResp); err == nil && len(lhbResp.Result.Data) > 0 {
			info := lhbResp.Result.Data[0]
			s.LHBNet = info.NetAmt

			netStr := fmt.Sprintf("%.1f万", info.NetAmt/10000)
			if math.Abs(info.NetAmt) > 100000000 {
				netStr = fmt.Sprintf("%.1f亿", info.NetAmt/100000000)
			}

			s.LHBInfo = fmt.Sprintf("%s 净:%s", info.Explain, netStr)
			return // Found latest
		}
	}
}

// 🆕 根据名称搜索股票代码
func SearchStock(keyword string) (string, string) {
	escaped := url.QueryEscape(keyword)
	url := fmt.Sprintf("http://searchapi.eastmoney.com/api/suggest/get?input=%s&type=14&token=D43BF722C8E33BDC906FB84D85E326E8", escaped)

	// Retry logic: 3 attempts
	for i := 0; i < 3; i++ {
		client := http.Client{Timeout: 10 * time.Second} // Increased timeout to 10s
		resp, err := client.Get(url)
		if err != nil {
			if i == 2 {
				fmt.Printf("SearchStock error (attempt %d): %v\n", i+1, err)
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		body, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		var searchResp struct {
			QuotationCodeTable struct {
				Data []struct {
					Code string `json:"Code"`
					Name string `json:"Name"`
					Mkt  string `json:"MarketType"`
				} `json:"Data"`
			} `json:"QuotationCodeTable"`
		}

		if err := json.Unmarshal(body, &searchResp); err == nil {
			if len(searchResp.QuotationCodeTable.Data) > 0 {
				match := searchResp.QuotationCodeTable.Data[0]
				return match.Code, match.Name
			}
		}
		// If unmarshal fail or empty data, maybe retry?
		// For now assume empty data means not found.
		if len(searchResp.QuotationCodeTable.Data) == 0 && i < 2 {
			// Maybe sporadic empty? Retry.
			time.Sleep(200 * time.Millisecond)
			continue
		}
		break
	}
	return "", ""
}
