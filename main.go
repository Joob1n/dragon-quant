package main

import (
	"dragon-quant/config"
	core "dragon-quant/core/analysis_all_stocks"
	"dragon-quant/core/analysis_special_stocks/hold_kline"
	"dragon-quant/output_formatter"
	"flag"
	"fmt"
)

var holdKlineMode = flag.Bool("hold-kline", false, "Run Hold Kline Processor only")
var reviewDays = flag.Int("days", 7, "Days for hold review (1 or 7)")

func main() {
	fmt.Println(`
   ___  ____    _    ____  ____  _   _ 
  / _ \|  _ \  / \  / ___|/ _ \| \ | |
 | | | | |_) |/ _ \| |  _| | | |  \| |
 | |_| |  _ <| ___ | |_| | |_| | |\  |
  \___/|_| \_/_/   \_\____|\___/|_| \_| v10.5
   Apocalypse: Memory + VWAP + LHB + Old Fox + Hold-Kline
	`)

	flag.Parse()

	// Load Config Early
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("⚠️ 加载 config.yaml 失败: %v\n", err)
		return
	}

	if *holdKlineMode {
		analysisSpecialStocks(cfg)
	} else {
		analysisAllStocks(cfg)
	}
}

func analysisAllStocks(cfg *config.Config) {
	// Public variables for Report Generation

	// --- Step 1: 扫描热点 ---
	scanHotPointSectorsResult := core.ScanHotPointSectors(cfg)

	// --- Step 2: 竞价与资金初筛 ---
	findCandidatesResult := core.FindCandidates(cfg, scanHotPointSectorsResult)

	// --- Step 3: 深度技术 + 龙头地位推演 ---
	inferStockLeadersResult := core.InferStockLeaders(cfg, findCandidatesResult)

	// --- Step 4: 输出 ---

	if len(inferStockLeadersResult.FinalPool) > 0 {
		output_formatter.PrintDragonTable(inferStockLeadersResult.FinalPool)
		output_formatter.GenFiles(cfg, scanHotPointSectorsResult.AllSectors,
			inferStockLeadersResult.FinalPool, inferStockLeadersResult.Elapsed,
			scanHotPointSectorsResult.SentimentStr)

		// --- Step 6: DeepSeek 老狐狸鉴股 (V10.4 Full Scan) ---
		findWinnersResult := core.FindWinners(cfg, scanHotPointSectorsResult, inferStockLeadersResult)

		output_formatter.PrintRiskReport(findWinnersResult.RiskResults)

		// Generate MD5
		output_formatter.WriteMD(cfg.ReportTop3FileMD, findWinnersResult.Top3MdBuffer.String())
		output_formatter.WriteMD(cfg.ReportTop1FileMD, findWinnersResult.Top1MdBuffer.String())
		output_formatter.WriteMD(cfg.ReportWinnersFileMD, findWinnersResult.WinnersMdBuffer.String())
		// Generate HTML
		output_formatter.SimpleMDToHTMLFile(cfg.ReportTop3FileMD, cfg.ReportTop3FileHTML)
		output_formatter.SimpleMDToHTMLFile(cfg.ReportTop1FileMD, cfg.ReportTop1FileHTML)
		output_formatter.SimpleMDToHTMLFile(cfg.ReportWinnersFileMD, cfg.ReportWinnersFileHTML)

		fmt.Printf("✅ 老狐狸报告(HTML)已更新: %s\n", cfg.ReportWinnersFileHTML)

	} else {
		fmt.Println("❌ 无符合条件的标的。")
	}
}

func analysisSpecialStocks(cfg *config.Config) {
	fmt.Println("🛡️ 启动持仓 30m K线深度审视模式...")

	processor := hold_kline.NewHoldProcessor(cfg.DeepSeek.APIKey)
	defer processor.Close()

	processor.Run(cfg, *reviewDays)
}
