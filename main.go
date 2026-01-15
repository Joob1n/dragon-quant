package main

import (
	"dragon-quant/config"
	"dragon-quant/data_processor"
	"dragon-quant/deepseek_reviewer"
	"dragon-quant/fetcher"
	"dragon-quant/hold_kline"
	"dragon-quant/model"
	"dragon-quant/output_formatter"
	"flag"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var holdKlineMode = flag.Bool("hold-kline", false, "Run Hold Kline Processor only")
var reviewDays = flag.Int("days", 7, "Days for hold review (1 or 7)")

func main() {
	flag.Parse()

	// Load Config Early
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("⚠️ 加载 config.yaml 失败: %v\n", err)
		return
	}

	if *holdKlineMode {
		fmt.Println("🛡️ 启动持仓 30m K线深度审视模式...")
		// TODO: Load from file or args? User said "array inside".
		// Sample list. User can edit this.
		holdStocks := []string{
			/*"圣达生物",
			"四川黄金",
			"云天化",
			"中钨高新",
			"宏创控股",
			"常山北明",
			"山东黄金",
			"中科金财",
			"浙数文化",
			"航天电子",
			"航天发展",
			"航天彩虹",*/
			"七一二",
			"鸿博股份",
			"国联股份",
			"中科软",
			"中贝通信",
		}

		processor := hold_kline.NewHoldProcessor(cfg.DeepSeek.APIKey)
		defer processor.Close()

		processor.Run(holdStocks, *reviewDays)
		return
	}

	start := time.Now()
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fileTime := time.Now().Format("2006-01-02-15")

	fmt.Println(`
   ___  ____    _    ____  ____  _   _ 
  / _ \|  _ \  / \  / ___|/ _ \| \ | |
 | | | | |_) |/ _ \| |  _| | | |  \| |
 | |_| |  _ <| ___ | |_| | |_| | |\  |
  \___/|_| \_/_/   \_\____|\___/|_| \_| v10.5
   Apocalypse: Memory + VWAP + LHB + Old Fox + Hold-Kline
`)

	// Public variables for Report Generation
	sectorTrendResults := make(map[string]deepseek_reviewer.SectorTrendResult)
	sectorNames := make(map[string]string)

	// --- Step 1: 扫描热点 ---
	fmt.Println("📡 [Step 1] 扫描全市场热点 (行业+概念)...")
	var allSectors []model.SectorInfo
	inds := fetcher.FetchTopSectors("m:90+t:2", data_processor.TopN, "行业")
	concepts := fetcher.FetchTopSectors("m:90+t:3", data_processor.TopN, "概念")
	allSectors = append(allSectors, inds...)
	allSectors = append(allSectors, concepts...)
	fmt.Printf("   -> 锁定板块: %d 个\n", len(allSectors))

	// 🆕 [Step 1.2] AI Sector Filter (DeepSeek)
	// Only run if API Key is present
	if cfg.DeepSeek.APIKey != "" {
		fmt.Println("🧠 [Step 1.2] 启动 AI 板块主力意图识别 (DeepSeek)...")

		// 1. Fetch History for all sectors
		var validSectors []model.SectorInfo
		// 🆕 Capture results for report (Vars declared at func top)

		fmt.Printf("   -> 正在获取 %d 个板块的 K线数据...\n", len(allSectors))
		for i := range allSectors {
			// Fetch History
			// Use pointer to modify directly? No, range returns copy.
			// Let's just modify the item and append to validSectors
			s := allSectors[i]
			s.History = fetcher.FetchSectorHistory(s.Code)

			// Populate Name in Kline (User Request)
			for k := range s.History {
				s.History[k].Name = s.Name
			}

			if len(s.History) > 5 {
				validSectors = append(validSectors, s)
			}
		}

		// 2. Call AI Review
		reviewer := deepseek_reviewer.NewReviewer(cfg.DeepSeek.APIKey)
		aiResults := reviewer.ReviewSectorTrends(validSectors)
		sectorTrendResults = aiResults // Save for later

		// Save names
		for _, s := range validSectors {
			sectorNames[s.Code] = s.Name
		}

		// 3. Filter
		var finalSectors []model.SectorInfo
		dumpCount := 0

		fmt.Println("\n🔍 AI 板块筛选结果:")
		for _, s := range validSectors {
			if res, ok := aiResults[s.Code]; ok {
				s.AIView = res.Status
				s.AIReason = res.Reason

				// Logic: Reject "Dump" or "Bearish"
				if res.Status == "Dump" {
					fmt.Printf("   ❌ 剔除 [%s]: %s (原因: %s)\n", s.Name, res.Status, res.Reason)
					dumpCount++
					continue
				}

				// Keep others (MainWave, Wash, Ignition)
				icon := "✅"
				if res.Status == "MainWave" {
					icon = "🚀"
				} else if res.Status == "Wash" {
					icon = "🛁"
				}
				fmt.Printf("   %s 保留 [%s]: %s\n", icon, s.Name, res.Status)
				finalSectors = append(finalSectors, s)

			} else {
				// AI Failed or no result? Keep purely based on technicals?
				// For safety, let's keep but mark unknown.
				finalSectors = append(finalSectors, s)
			}
		}
		fmt.Printf("   -> AI 剔除: %d 个, 最终保留: %d 个\n", dumpCount, len(finalSectors))
		allSectors = finalSectors
	} else {
		fmt.Println("⚠️ 未配置 API Key, 跳过 AI 板块筛选。")
	}

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
		// cfg already loaded at start

		// --- Step 6: DeepSeek 老狐狸鉴股 (V10.4 Full Scan) ---
		// cfg already loaded at start
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

				// 🆕 Fetch Market Context (Global)
				fmt.Println("🌡️ [Step 6.0] 获取大盘 (000001) 7日30分钟走势作为全局背景...")
				marketContext := fetcher.FetchMarket30mKline(7)
				if marketContext == "" {
					fmt.Println("⚠️ [Step 6.0] 获取大盘数据失败或为空！(AI 将缺失全局视野)")
				} else {
					fmt.Printf("✅ [Step 6.0] 大盘数据获取成功 (长度: %d chars)\n", len(marketContext))
				}

				// Generate Markdown Report Base
				reportFileMD := fmt.Sprintf("DeepSeek_Fox_Report_%s.md", fileTime)
				reportFileHTML := fmt.Sprintf("DeepSeek_Fox_Report_%s.html", fileTime)
				var mdBuffer strings.Builder
				mdBuffer.WriteString("# 🦊 DeepSeek 老狐狸板块博弈报告\n")
				mdBuffer.WriteString(fmt.Sprintf("**生成时间**: %s\n\n", timestamp))
				mdBuffer.WriteString("> **战略**: 30m结构优先 -> 老狐狸博弈复审 -> 总决赛。\n\n")

				// 🆕 Step 6.0: AI Sector Trends Report (All Scanned Sectors)
				if len(sectorTrendResults) > 0 {
					mdBuffer.WriteString("## 🔭 主力意图识别 (Sector Trends)\n")
					mdBuffer.WriteString("> **逻辑**: 基于日线K线形态，识别主力是洗盘(Wash)、主升(MainWave)还是出货(Dump)。\n\n")

					// Sort keys
					var sortedCodes []string
					for k := range sectorTrendResults {
						sortedCodes = append(sortedCodes, k)
					}
					sort.Strings(sortedCodes)

					for _, code := range sortedCodes {
						res := sectorTrendResults[code]
						name := sectorNames[code]
						if name == "" {
							name = code
						}

						icon := "❓"
						desc := "未知"
						if res.Status == "MainWave" {
							icon = "🚀"
							desc = "主升浪 (MainWave)"
						} else if res.Status == "Wash" {
							icon = "🛁"
							desc = "洗盘/分歧 (Wash)"
						} else if res.Status == "Ignition" {
							icon = "🔥"
							desc = "启动 (Ignition)"
						} else if res.Status == "Dump" {
							icon = "❌"
							desc = "出货/下跌 (Dump)"
						}

						mdBuffer.WriteString(fmt.Sprintf("**%s %s** (%s) - %s\n", icon, name, code, desc))
						mdBuffer.WriteString(fmt.Sprintf("> %s\n\n", res.Reason))
					}
					mdBuffer.WriteString("---\n")
				}

				// 🆕 Step 6.1: 30分钟结构 AI 专项审视 (Pre-Filter)
				fmt.Println("\n🧠 [Step 6.1] 启动 30分钟结构大师 (筛选 Top 3)...")
				res30m := reviewer.ReviewBySector30m(sectorStocks)

				// Filtered stocks for Old Fox (Only Top 3 from 30m)
				foxInput := make(map[string][]*model.StockInfo)

				if len(res30m) > 0 {
					mdBuffer.WriteString("\n# 🛠️ 30分钟结构精选 (Top 3)\n")
					mdBuffer.WriteString("> **逻辑**: 识别 N字反包、空中加油、双底等形态。\n\n")

					// Sort sectors
					var sectors30m []string
					for s := range res30m {
						sectors30m = append(sectors30m, s)
					}
					sort.Strings(sectors30m)

					for _, secName := range sectors30m {
						res := res30m[secName]
						if len(res.Top3) == 0 {
							continue
						}
						mdBuffer.WriteString(fmt.Sprintf("## %s\n", secName))
						for _, t := range res.Top3 {
							icon := "🔹"
							if t.Rank == 1 {
								icon = "🥇"
							} else if t.Rank == 2 {
								icon = "🥈"
							} else if t.Rank == 3 {
								icon = "🥉"
							}

							mdBuffer.WriteString(fmt.Sprintf("%s **%s** (%s) - %s\n", icon, t.StockName, t.StockCode, t.Metric))
							mdBuffer.WriteString(fmt.Sprintf("> **分析**: %s\n", t.Reason))
							mdBuffer.WriteString(fmt.Sprintf("> **推演**: %s\n\n", t.Deduction))

							// Add to Fox Input
							// Find the original stock info object
							for _, original := range sectorStocks[secName] {
								if original.Code == t.StockCode {
									foxInput[secName] = append(foxInput[secName], original)
									break
								}
							}
						}
						mdBuffer.WriteString("---\n")
					}
					fmt.Println("✅ 30分钟结构分析完成，MD已暂存。")
				}

				// Write intermediate report
				output_formatter.WriteMD(reportFileMD, mdBuffer.String())

				// 🆕 Step 6.2: Old Fox Review (Only on 30m Top 3)
				fmt.Printf("\n🦊 [Step 6.2] 老狐狸博弈复审 (入围 %d 个板块)...\n", len(foxInput))
				sectorResults := reviewer.ReviewBySector(foxInput, marketContext)

				mdBuffer.WriteString("\n# 🦊 老狐狸复审 & 板块王者\n")

				// Iterate Sectors (Sorted)
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
					for _, stock := range foxInput[secName] {
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

				// Update Report
				output_formatter.WriteMD(reportFileMD, mdBuffer.String())

				// --- Step 7: Grand Final (Top 5) ---
				fmt.Println("\n🏆 [Step 7] 启动总决赛 (Top 5 巅峰对决)...")
				// ... (Rest of Step 7 remains, but using sectorResults which is filtered)

				// 1. Collect Candidates
				var grandCandidates []*model.StockInfo
				for _, r := range sectorResults {
					if r.FinalPick != nil {
						for _, s := range foxInput[r.SectorName] {
							if s.Code == r.FinalPick.StockCode {
								grandCandidates = append(grandCandidates, s)
								break
							}
						}
					}
				}

				// ... (Grand Final Logic)
				if len(grandCandidates) > 0 {
					gfRes := reviewer.ReviewGrandFinals(grandCandidates, marketContext)
					if gfRes != nil {
						mdBuffer.WriteString("\n\n# 🏆 总决赛：五虎上将 (Grand Final Top 5)\n")
						mdBuffer.WriteString(fmt.Sprintf("> **市场情绪**: %s\n\n", gfRes.MarketSentiment))

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

							mdBuffer.WriteString(fmt.Sprintf("### %s: %s (%s)\n", icon, t.StockName, t.StockCode))
							mdBuffer.WriteString(fmt.Sprintf("> %s\n\n", t.Reason))
						}
					}
				} else {
					fmt.Println("🤷‍♂️ 没有产生任何板块龙头，取消总决赛。")
					mdBuffer.WriteString("\n\n# 🤷‍♂️ 总决赛取消\n> 原因: 没有产生任何符合条件的板块龙头。")
				}

				output_formatter.WriteMD(reportFileMD, mdBuffer.String())
				// Generate HTML
				output_formatter.SimpleMDToHTMLFile(reportFileMD, reportFileHTML)
				fmt.Printf("✅ 老狐狸报告(HTML)已更新: %s\n", reportFileHTML)

			}
		} else {
			fmt.Println("\n⚠️ [Step 6] 未配置 DEEPSEEK_API_KEY，跳过 AI 点评。")
		}

	} else {
		fmt.Println("❌ 无符合条件的标的。")
	}
}
