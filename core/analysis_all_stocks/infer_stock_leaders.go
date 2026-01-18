package core

import (
	"dragon-quant/config"
	"dragon-quant/data_processor"
	"dragon-quant/fetcher"
	"dragon-quant/model"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type InferStockLeadersResult struct {
	FinalPool []*model.StockInfo
	Elapsed   time.Duration
}

func InferStockLeaders(cfg *config.Config, findCandidatesResult FindCandidatesResult) InferStockLeadersResult {
	fmt.Println("🔬 [Step 3] 计算技术指标 & 推演龙头地位...")

	var mu sync.Mutex

	var finalPool []*model.StockInfo
	var techWg sync.WaitGroup
	sem := make(chan struct{}, 20)

	for _, stk := range findCandidatesResult.Candidates {
		techWg.Add(1)
		go func(s *model.StockInfo) {
			defer techWg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// 1. 龙头地位推演 (基于板块标签)
			data_processor.InferDragonStatus(s)

			// 2. K线计算
			klines := fetcher.FetchHistoryData(s.Code, 60)
			if len(klines) < 30 {
				return
			}

			// 🆕 3. 深度数据 (竞价 f277 + 盘口 + 龙虎榜)
			// 注意：fetchStockDetails 会更新 s 中的 CallAuctionAmt 等字段
			fetcher.FetchStockDetails(s)

			if s.ChangePct > 7.0 || s.CallAuctionAmt > 50000000 {
				fetcher.FetchLHBData(s)
			}

			// 🆕 计算开盘承接率 (Sustainability)
			// 注意: Fetch5MinKline 使用 fields=f57(AvgAmt?) no, Amount.
			kline5 := fetcher.Fetch5MinKline(s.Code)
			s.OpenVolRatio = data_processor.CalculateSustainability(s.CallAuctionAmt, kline5)

			// 🆕 30分钟级别主力意图 (从30m K线挖掘)
			klines30m := fetcher.Fetch30MinKline(s.Code, 60)
			s.Note30m = data_processor.Analyze30mStrategy(klines30m)

			// 🆕 Format 30m K-lines for AI (Last 12 bars = 1.5 days)
			var sb strings.Builder
			count30m := len(klines30m)
			startIdx := 0
			if count30m > 12 {
				startIdx = count30m - 12
			}
			for i := startIdx; i < count30m; i++ {
				k := klines30m[i]
				// 简化的K线描述: C=Close, V=Amount, R=Rate
				rate := 0.0
				if i > 0 {
					prev := klines30m[i-1].Close
					if prev > 0 {
						rate = (k.Close - prev) / prev * 100
					}
				}
				sb.WriteString(fmt.Sprintf("[Bar-%d: C=%.2f, R=%.2f%%, V=%.0f] ", i-startIdx+1, k.Close, rate, k.Amount))
			}
			s.KLine30mStr = sb.String()

			// 🆕 4. 深度K线挖掘 (VWAP + 记忆)
			s.VWAP, s.ProfitDev = data_processor.CalculateVWAP(klines, 30, s.Price)
			s.DragonHabit = data_processor.AnalyzeDragonHabit(klines)

			s.MA5, s.MA20 = data_processor.CalculateMA(klines)
			s.DIF, s.DEA, s.Macd = data_processor.CalculateMACD(klines)
			s.RSI6 = data_processor.CalculateRSI(klines, 6)

			// 3. 技术备注构造 + 4. 终极过滤
			passed := data_processor.GenerateTechNotes(s)

			if passed {
				mu.Lock()
				finalPool = append(finalPool, s)
				mu.Unlock()
			}
		}(stk)
	}
	techWg.Wait()

	// 排序：按竞价金额 (OpenAmt) 降序 -> 谁是开盘之王
	// 排序：按真实竞价金额 (CallAuctionAmt) 降序
	sort.Slice(finalPool, func(i, j int) bool {
		return finalPool[i].CallAuctionAmt > finalPool[j].CallAuctionAmt
	})

	elapsed := time.Since(cfg.StartTime)
	fmt.Printf("\n🏁 扫描完成! 耗时: %s | 最终入选: %d 只\n", elapsed, len(finalPool))

	return InferStockLeadersResult{
		FinalPool: finalPool,
		Elapsed:   elapsed,
	}
}
