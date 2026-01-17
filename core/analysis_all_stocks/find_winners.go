package core

import (
	"dragon-quant/ai_reviewer/deepseek_reviewer"
	"dragon-quant/config"
	"dragon-quant/data_processor"
	"dragon-quant/fetcher"
	"dragon-quant/model"
	"fmt"
	"sort"
	"strings"
)

type FindWinnersResult struct {
	RiskResults          []model.RiskResult
	SectorStatusMdBuffer strings.Builder
	Top3MdBuffer         strings.Builder
	Top1MdBuffer         strings.Builder
	WinnersMdBuffer      strings.Builder
}

var findWinnersResult FindWinnersResult

func FindWinners(cfg *config.Config,
	scanHotPointSectorsResult ScanHotPointSectorsResult,
	inferStockLeadersResult InferStockLeadersResult) FindWinnersResult {

	sectorStocks := getStocksGroupBySector(inferStockLeadersResult)

	apiKey := cfg.DeepSeek.APIKey
	if apiKey != "" {
		fmt.Println("\n🧠 [Step 6] 呼叫 DeepSeek 老狐狸 (全量审视)...")

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
			initSectorStatus(cfg, scanHotPointSectorsResult)
			foxInput := findTop3ForEachSector(cfg, reviewer, sectorStocks)
			sectorResults := findWinnerForEachSector(cfg, reviewer, foxInput, marketContext)
			findTheUltimateWinners(cfg, reviewer, sectorResults, foxInput, marketContext)
		}
	} else {
		fmt.Println("\n⚠️ [Step 6] 未配置 DEEPSEEK_API_KEY，跳过 AI 点评。")
	}

	return findWinnersResult
}

func getStocksGroupBySector(inferStockLeadersResult InferStockLeadersResult) map[string][]*model.StockInfo {
	// --- Step 5: 二次风控筛选 (老狐狸逻辑) ---
	fmt.Println("\n🦊 [Step 5] 启动老狐狸二次风控筛选...")
	riskConfig := data_processor.NewRiskConfig()
	riskResults := data_processor.RiskScreen(inferStockLeadersResult.FinalPool, riskConfig)

	findWinnersResult.RiskResults = riskResults

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
	return sectorStocks
}

func initSectorStatus(cfg *config.Config,
	scanHotPointSectorsResult ScanHotPointSectorsResult) {
	var mdBuffer strings.Builder
	mdBuffer.WriteString("# 🦊 DeepSeek 老狐狸板块博弈报告\n")
	mdBuffer.WriteString(fmt.Sprintf("**生成时间**: %s\n\n", cfg.StartTsStr))
	mdBuffer.WriteString("> **战略**: 30m结构优先 -> 老狐狸博弈复审 -> 总决赛。\n\n")

	// 🆕 Step 6.0: AI Sector Trends Report (All Scanned Sectors)
	if len(scanHotPointSectorsResult.SectorTrendResults) > 0 {
		mdBuffer.WriteString("## 🔭 主力意图识别 (Sector Trends)\n")
		mdBuffer.WriteString("> **逻辑**: 基于日线K线形态，识别主力是洗盘(Wash)、主升(MainWave)还是出货(Dump)。\n\n")

		// Sort keys
		var sortedCodes []string
		for k := range scanHotPointSectorsResult.SectorTrendResults {
			sortedCodes = append(sortedCodes, k)
		}
		sort.Strings(sortedCodes)

		for _, code := range sortedCodes {
			res := scanHotPointSectorsResult.SectorTrendResults[code]
			name := scanHotPointSectorsResult.SectorNames[code]
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

	findWinnersResult.SectorStatusMdBuffer = mdBuffer
}

func findTop3ForEachSector(cfg *config.Config,
	reviewer *deepseek_reviewer.Reviewer,
	sectorStocks map[string][]*model.StockInfo) map[string][]*model.StockInfo {
	var mdBuffer strings.Builder

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

	findWinnersResult.Top3MdBuffer = mdBuffer
	return foxInput
}

func findWinnerForEachSector(cfg *config.Config,
	reviewer *deepseek_reviewer.Reviewer,
	foxInput map[string][]*model.StockInfo,
	marketContext string) map[string]*deepseek_reviewer.SectorResult {
	var mdBuffer strings.Builder

	// 🆕 Step 6.2: Old Fox Review (Only on 30m Top 3)
	fmt.Printf("\n🦊 [Step 6.2] 老狐狸博弈复审 (入围 %d 个板块)...\n", len(foxInput))
	sectorResults := reviewer.ReviewBySector(foxInput, marketContext)

	mdBuffer.WriteString("\n# 🦊 老狐狸复审 & 板块王者 Top1\n")

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

	findWinnersResult.Top1MdBuffer = mdBuffer

	return sectorResults
}

func findTheUltimateWinners(cfg *config.Config,
	reviewer *deepseek_reviewer.Reviewer,
	sectorResults map[string]*deepseek_reviewer.SectorResult,
	foxInput map[string][]*model.StockInfo,
	marketContext string) {
	var mdBuffer strings.Builder

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

	findWinnersResult.WinnersMdBuffer = mdBuffer
}
