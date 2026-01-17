package core

import (
	"dragon-quant/ai_reviewer/deepseek_reviewer"
	"dragon-quant/config"
	"dragon-quant/data_processor"
	"dragon-quant/fetcher"
	"dragon-quant/model"
	"fmt"
)

type ScanHotPointSectorsResult struct {
	AllSectors         []model.SectorInfo
	SentimentStr       string
	SectorTrendResults map[string]deepseek_reviewer.SectorTrendResult
	SectorNames        map[string]string
}

func ScanHotPointSectors(cfg *config.Config) ScanHotPointSectorsResult {
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

	return ScanHotPointSectorsResult{
		AllSectors:         allSectors,
		SentimentStr:       sentimentStr,
		SectorTrendResults: sectorTrendResults,
		SectorNames:        sectorNames,
	}
}
