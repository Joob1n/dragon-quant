package hold_kline

import (
	"dragon-quant/ai_reviewer/deepseek_reviewer"
	"dragon-quant/config"
	"dragon-quant/data_processor"
	"dragon-quant/fetcher"
	"dragon-quant/model"
	"fmt"
	"strings"
	"sync"
	"time"
)

type HoldProcessor struct {
	Reviewer *deepseek_reviewer.Reviewer
}

type StockResult struct {
	Code      string
	Name      string
	Tags      []string
	KLine30m  string // Kept for compatibility or debug
	AIReview  string
	TechNotes string
}

func NewHoldProcessor(apiKey string) *HoldProcessor {
	return &HoldProcessor{
		Reviewer: deepseek_reviewer.NewReviewer(apiKey),
	}
}

func (p *HoldProcessor) Close() {
	// No global resources to close anymore
}

// Run performs a review for the specified number of days
func (p *HoldProcessor) Run(cfg *config.Config, days int) {

	names := cfg.HoldStocks
	fmt.Printf("\n�️ [Custom Review] Starting for %d stocks (Days=%d)...\n", len(names), days)

	p.runGeneric(cfg, days)
}

func (p *HoldProcessor) runGeneric(cfg *config.Config, days int) {
	var results []StockResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	names := cfg.HoldStocks

	// Semaphore to limit concurrency (DeepSeek API limits)
	maxConcurrent := 5
	sem := make(chan struct{}, maxConcurrent)

	fmt.Printf("🚀 Launching %d goroutines (Max Parallel: %d)...\n", len(names), maxConcurrent)

	for _, nameInput := range names {
		wg.Add(1)

		go func(nameIn string) {
			defer wg.Done()

			// Acquire token
			sem <- struct{}{}
			defer func() { <-sem }()

			// --- Isolated Execution Context ---

			// 0. Init Thread-Local DuckDB
			// Critical for data isolation (kline_1m table)
			duck, err := data_processor.NewDuckDB("")
			if err != nil {
				fmt.Printf("❌ [%s] DuckDB Init Failed: %v\n", nameIn, err)
				return
			}
			defer duck.Close()

			klineProc := data_processor.NewKlineProcessor(duck)

			// 1. Resolve Code
			// fmt.Printf("   -> Searching %s ... ", nameIn) // Avoid noisy interleaved logs
			code, realName := fetcher.SearchStock(nameIn)
			if code == "" {
				fmt.Printf("❌ [%s] Not Found.\n", nameIn)
				return
			}

			// 2. Fetch 1m K-line (Retry 5 times)
			var klines []model.KLineData
			for retry := 0; retry < 5; retry++ {
				klines = fetcher.Fetch1MinKline(code, days)
				if len(klines) > 0 {
					break
				}
				if retry < 4 {
					time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond) // Backoff
					fmt.Printf("🔄 [%s] Retry fetching data (%d/5)...\n", realName, retry+1)
				}
			}

			if len(klines) == 0 {
				fmt.Printf("⚠️ [%s] No Data after 5 attempts. Skipping.\n", realName)
				return
			}
			fmt.Printf("✅ [%s] Got %d bars.\n", realName, len(klines))

			// 3. Load into DuckDB
			err = klineProc.LoadData(klines)
			if err != nil {
				fmt.Printf("❌ [%s] DuckDB Load Error: %v\n", realName, err)
				return
			}

			// 4. Advanced Analysis (Aggregation + Anomaly)
			events, err := klineProc.AnalyzeVolatility()
			if err != nil {
				fmt.Printf("❌ [%s] Analysis Error: %v\n", realName, err)
				return
			}

			// 5. Build Context Prompt
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("以下是 %s (%s) 基于“%d天 1分钟高频数据”聚合挖掘出的【关键异动时刻】：\n\n", realName, code, days))
			sb.WriteString("> **数据说明**:\n")
			sb.WriteString("> `VolRatio`: 量比 (当前量/1小时均量)\n")
			sb.WriteString("> `Pos`: 在当前30分钟K线内的相对位置 (0=Low, 1=High, >1=Breakout)\n")
			sb.WriteString("> `Bias`: 相对30分钟均价(VWAP)的乖离率\n\n")

			for i, e := range events {
				sb.WriteString(fmt.Sprintf("**Event %d**: %s | Price: %.2f | %s\n",
					i+1, e.Time.Format("15:04"), e.Price, e.Note))
				sb.WriteString(fmt.Sprintf("- 📊 Stats: VolRatio=%.1fx, Pos=%.2f, Bias=%.2f%%\n",
					e.VolRatio, e.RelativePos, e.Bias30m))
				sb.WriteString("\n")
			}

			contextStr := sb.String()

			// 6. AI Analysis
			prompt := fmt.Sprintf(`
# Role: 顶级游资操盘手 (刀口舔血、博弈大师)
# Task: 基于盘口微观异动，通过“情绪”与“筹码”双重视角，复盘主力操盘意图。我们需要明天强行上车，做各种短线操作。我们是顶级的赌徒。
# Stock: %s (%s)
# Context: 过去 %d 天的高频博弈数据。

# Data Provided (DuckDB 异动挖掘):
- **异动时刻**: 资金疯狂进攻或砸盘的瞬间。
- **VolRatio (量比)**: 突发资金强度 ( > 3x 为异动, > 5x 为抢筹/出货)。
- **Pos (相对位置)**: 30分钟K线内的身位 (0=底, 1=顶, >1=突破)。
- **Bias (乖离)**: 偏离30分钟均价的幅度，极大乖离往往意味着反转或爆发。

%s

# Analysis Requirements:
1. **主力身份侧写**: 是“解放南路”式的暴力拉升，还是“温州帮”式的出货？是“机构”在维护，还是“散户”在踩踏？
2. **杀伐决断**:
   - **刀口**: 哪里是风险释放的极致低点？
   - **博弈**: 哪里是情绪一致的高潮点？
3. **操作指令 (Direct Command)**:
   - 必须给出明确的买点。以及各种相应的指标的具体操作。我需要你给出操作锦囊。
   - 附带一句话犀利点评 (Stylized: 简短、冷酷、一针见血)。

请用游资的口吻，不要废话，直击灵魂。
`, realName, code, days, contextStr)

			history := []deepseek_reviewer.Message{
				{Role: "user", Content: prompt},
			}

			// Debug: Print Prompt (Atomic Print to avoid mess)
			// fmt.Printf("\n--- [Debug %s] Prompt ---\n%s\n", realName, contextStr)

			fmt.Printf("🧠 [%s] Analyzing (%d Events)...\n", realName, len(events))
			review := p.Reviewer.SendChat(history)

			// Debug: Log raw review length and preview
			fmt.Printf("📝 [%s] DeepSeek Resp Len: %d. Preview: %s...\n",
				realName, len(review), strings.ReplaceAll(review[:min(len(review), 50)], "\n", " "))

			fmt.Printf("✅ [%s] Done. Appending Result.\n", realName)

			// Collect Result safely
			mu.Lock()
			results = append(results, StockResult{
				Code:     code,
				Name:     realName,
				KLine30m: contextStr,
				AIReview: review,
			})
			mu.Unlock()

		}(nameInput)
	}

	wg.Wait()
	// Generate HTML (Reuse existing generic generator)
	fmt.Printf("📊 Generating Report for %d results...\n", len(results))
	GenerateHoldReport(cfg, results)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
