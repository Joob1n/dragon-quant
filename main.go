package main

import (
	"dragon-quant/config"
	"dragon-quant/data_processor"
	"dragon-quant/deepseek_reviewer"
	"dragon-quant/fetcher"
	"dragon-quant/model"
	"dragon-quant/output_formatter"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

func main() {
	start := time.Now()
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fileTime := time.Now().Format("2006-01-02-15")

	fmt.Println(`
   ___  ____    _    ____  ____  _   _ 
  / _ \|  _ \  / \  / ___|/ _ \| \ | |
 | | | | |_) |/ _ \| |  _| | | |  \| |
 | |_| |  _ <| ___ | |_| | |_| | |\  |
  \___/|_| \_/_/   \_\____|\___/|_| \_| v10.4
   Apocalypse: Memory + VWAP + LHB + Old Fox
`)

	// --- Step 1: 扫描热点 ---
	fmt.Println("📡 [Step 1] 扫描全市场热点 (行业+概念)...")
	var allSectors []model.SectorInfo
	inds := fetcher.FetchTopSectors("m:90+t:2", data_processor.TopN, "行业")
	concepts := fetcher.FetchTopSectors("m:90+t:3", data_processor.TopN, "概念")
	allSectors = append(allSectors, inds...)
	allSectors = append(allSectors, concepts...)
	fmt.Printf("   -> 锁定板块: %d 个\n", len(allSectors))

	// 🆕 Fetch Market Sentiment
	fmt.Println("🌡️ [Step 1.1] 探测市场情绪 (昨日涨停表现)...")
	sentimentVal := fetcher.FetchSentimentIndex()
	sentimentStr := data_processor.AnalyzeSentiment(sentimentVal)
	fmt.Printf("   -> 情绪指数: %.2f%% (%s)\n", sentimentVal, sentimentStr)

	// --- Step 2: 竞价与资金初筛 ---
	fmt.Println("🚀 [Step 2] 启动竞价资金初筛 (Price/Flow/CallAuction)...")

	candidates := make(map[string]*model.StockInfo)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, sec := range allSectors {
		wg.Add(1)
		go func(s model.SectorInfo) {
			defer wg.Done()
			// 🔥 f19:开盘金额(竞价), f62:净流入, f7:振幅
			stocks := fetcher.FetchSectorStocks(s.Code)

			for _, stk := range stocks {
				// Use the FilterBasic function
				if !data_processor.FilterBasic(stk) {
					continue
				}

				mu.Lock()
				if existing, exists := candidates[stk.Code]; exists {
					existing.Tags = append(existing.Tags, s.Name)
				} else {
					newStk := stk
					newStk.Tags = []string{s.Name}
					candidates[stk.Code] = &newStk
				}
				mu.Unlock()
			}
		}(sec)
	}
	wg.Wait()
	fmt.Printf("   -> 初筛入围: %d 只\n", len(candidates))

	// --- Step 3: 深度技术 + 龙头地位推演 ---
	fmt.Println("🔬 [Step 3] 计算技术指标 & 推演龙头地位...")

	var finalPool []*model.StockInfo
	var techWg sync.WaitGroup
	sem := make(chan struct{}, 20)

	for _, stk := range candidates {
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

	elapsed := time.Since(start)

	// --- Step 4: 输出 ---
	fmt.Printf("\n🏁 扫描完成! 耗时: %s | 最终入选: %d 只\n", elapsed, len(finalPool))

	if len(finalPool) > 0 {
		output_formatter.PrintDragonTable(finalPool)
		output_formatter.GenFiles(allSectors, finalPool, elapsed, sentimentStr)

		// --- Step 5: 二次风控筛选 (老狐狸逻辑) ---
		fmt.Println("\n🦊 [Step 5] 启动老狐狸二次风控筛选...")
		riskConfig := data_processor.NewRiskConfig()
		riskResults := data_processor.RiskScreen(finalPool, riskConfig)
		output_formatter.PrintRiskReport(riskResults)

		// --- Step 6: DeepSeek 老狐狸鉴股 (V10.4 Full Scan) ---
		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Printf("\n⚠️ [Step 6] 加载 config.yaml 失败: %v, 跳过 AI 点评。\n", err)
		} else {
			apiKey := cfg.DeepSeek.APIKey
			if apiKey != "" {
				fmt.Println("\n🧠 [Step 6] 呼叫 DeepSeek 老狐狸 (全量审视)...")

				// 准备全量数据 - Group by Sector
				sectorStocks := make(map[string][]*model.StockInfo)
				for _, r := range riskResults {
					// Use the first tag as Industry/Sector, default to "Unknown"
					sector := "其他板块"
					if len(r.Stock.Tags) > 0 {
						sector = r.Stock.Tags[0]
					}
					sectorStocks[sector] = append(sectorStocks[sector], r.Stock)
				}

				if len(sectorStocks) > 0 {
					reviewer := deepseek_reviewer.NewReviewer(apiKey)
					// Call the new Sector-based Review
					sectorResults := reviewer.ReviewBySector(sectorStocks)

					// Generate Markdown Report
					reportFileMD := fmt.Sprintf("DeepSeek_Fox_Report_%s.md", fileTime)
					reportFileHTML := fmt.Sprintf("DeepSeek_Fox_Report_%s.html", fileTime)

					var mdBuffer strings.Builder

					mdBuffer.WriteString("# 🦊 DeepSeek 老狐狸板块博弈报告\n")
					mdBuffer.WriteString(fmt.Sprintf("**生成时间**: %s\n\n", timestamp))
					mdBuffer.WriteString("> **战略**: 分板块弱肉强食，每个板块只选唯一真龙。\n\n")

					// Iterate Sectors (Sorted Order?)
					var sectors []string
					for s := range sectorResults {
						sectors = append(sectors, s)
					}
					sort.Strings(sectors)

					for _, secName := range sectors {
						res := sectorResults[secName]
						mdBuffer.WriteString(fmt.Sprintf("## 🛡️ 板块: %s\n", secName))

						// 1. Individual Reviews
						mdBuffer.WriteString("### 个股辣评\n")
						// Sort stocks in this sector for consistent order? (optional)
						// Let's iterate the original list order to match insertion
						for _, stock := range sectorStocks[secName] {
							if review, ok := res.StockReviews[stock.Code]; ok {
								mdBuffer.WriteString(fmt.Sprintf("- **%s**: %s\n", stock.Name, review))
							}
						}

						// 2. Final Pick
						mdBuffer.WriteString("\n### 👑 板块王者\n")
						if res.FinalPick != nil {
							fp := res.FinalPick
							mdBuffer.WriteString(fmt.Sprintf("#### 🎯 唯一指定标的：【%s / %s】\n\n", fp.StockName, fp.StockCode))
							mdBuffer.WriteString(fmt.Sprintf("**A. 嗜血逻辑**\n> %s\n\n", fp.Reason))
							mdBuffer.WriteString(fmt.Sprintf("**🔥 量化王牌**: `%s`\n\n", fp.KeyMetric))
							mdBuffer.WriteString("**B. 操盘策略**\n")
							mdBuffer.WriteString(fmt.Sprintf("- 🚀 **突击点位**: %s\n", fp.Strategy.EntryPrice))
							mdBuffer.WriteString(fmt.Sprintf("- 🛑 **熔断止损**: %s\n", fp.Strategy.StopLoss))
							mdBuffer.WriteString(fmt.Sprintf("- 💰 **获利了结**: %s\n\n", fp.Strategy.TargetPrice))
							mdBuffer.WriteString(fmt.Sprintf("**C. 盘中预警**: ⚠️ %s\n\n", fp.RiskWarning))
						} else {
							mdBuffer.WriteString("*(本板块无符合“必杀”标准的标的)*\n\n")
						}
						mdBuffer.WriteString("---\n")
					}

					// Save MD
					err := os.WriteFile(reportFileMD, []byte(mdBuffer.String()), 0644)
					if err == nil {
						fmt.Printf("\n✅ 老狐狸报告(MD)已生成: %s\n", reportFileMD)
					} else {
						fmt.Printf("❌ MD生成失败: %v\n", err)
					}

					// --- Step 7: Grand Final (Top 5) ---
					fmt.Println("\n🏆 [Step 7] 启动总决赛 (Top 5 巅峰对决)...")

					// 1. Collect Candidates (Sector Winners)
					var grandCandidates []*model.StockInfo
					for _, r := range sectorResults {
						if r.FinalPick != nil {
							// Find the StockInfo object
							// We don't have a direct map key for it easily, but we can browse sectorStocks
							// Optimization: store *StockInfo in SectorResult?
							// For now, loop sectorStocks[r.SectorName]
							for _, s := range sectorStocks[r.SectorName] {
								if s.Code == r.FinalPick.StockCode {
									grandCandidates = append(grandCandidates, s)
									break
								}
							}
						}
					}

					// 2. Run Review
					if len(grandCandidates) > 0 {
						gfRes := reviewer.ReviewGrandFinals(grandCandidates)
						if gfRes != nil {
							// Append to Report (Prepend or Append?)
							// Let's Append a "Grand Final" chapter
							var gfBuffer strings.Builder
							gfBuffer.WriteString("\n\n# 🏆 总决赛：五虎上将 (Grand Final Top 5)\n")
							gfBuffer.WriteString(fmt.Sprintf("> **市场情绪**: %s\n\n", gfRes.MarketSentiment))

							for _, t := range gfRes.Top5 {
								icon := "🎖️"
								if t.Rank == 1 {
									icon = "👑 榜首 (The King)"
								}
								if t.Rank == 2 || t.Rank == 3 {
									icon = "🛡️ 中军 (General)"
								}
								if t.Rank == 4 || t.Rank == 5 {
									icon = "⚔️ 前锋 (Vanguard)"
								}

								gfBuffer.WriteString(fmt.Sprintf("### %s: %s (%s)\n", icon, t.StockName, t.StockCode))
								gfBuffer.WriteString(fmt.Sprintf("> %s\n\n", t.Reason))
							}

							// Re-write file with appended content
							// actually, better to just modify mdBuffer before writing file?
							// But we already wrote it. Let's append.

							f, err := os.OpenFile(reportFileMD, os.O_APPEND|os.O_WRONLY, 0644)
							if err == nil {
								f.WriteString(gfBuffer.String())
								f.Close()
								fmt.Println("✅ 总决赛名单已追加至报告。")

								// Re-generate HTML with full content
								fullContent, _ := os.ReadFile(reportFileMD)
								htmlContent := output_formatter.SimpleMDToHTML(string(fullContent))
								os.WriteFile(reportFileHTML, []byte(htmlContent), 0644)
								fmt.Printf("✅ 老狐狸报告(HTML)已更新: %s\n", reportFileHTML)

							}
						}
					} else {
						fmt.Println("🤷‍♂️ 没有产生任何板块龙头，取消总决赛。")
					}
				}

			} else {
				fmt.Println("\n⚠️ [Step 6] 未配置 DEEPSEEK_API_KEY，跳过 AI 点评。")
			}
		}

	} else {
		fmt.Println("❌ 无符合条件的标的。")
	}
}
