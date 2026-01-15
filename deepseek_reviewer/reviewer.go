package deepseek_reviewer

import (
	"bytes"
	"dragon-quant/model"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Reviewer struct {
	APIKey string
	Client *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

const (
	DeepSeekAPIURL = "https://api.deepseek.com/chat/completions"
	ModelName      = "deepseek-chat"
)

type SniperJSON struct {
	StockName string `json:"stock_name"`
	StockCode string `json:"stock_code"`
	Reason    string `json:"reason"`
	KeyMetric string `json:"key_metric"`
	Strategy  struct {
		EntryPrice  string `json:"entry_price"`
		StopLoss    string `json:"stop_loss"`
		TargetPrice string `json:"target_price"`
	} `json:"strategy"`
	RiskWarning string `json:"risk_warning"`
}

type SectorResult struct {
	SectorName   string
	StockReviews map[string]string
	FinalPick    *SniperJSON
}

const SniperPrompt = `# Role: 顶级短线操盘大师 / 敢死队总舵主

1. 任务背景
现在是实盘博弈时刻。你必须利用之前的分析，从当前板块中选出【唯一】一个确定性最高的标的。
禁止模棱两可，禁止空仓建议。

2. 输出要求 (严格执行)
请仅返回一个标准的 JSON 对象，不要包含任何 Markdown 格式（如 json code blocks），不要包含任何额外的解释文字。
JSON 格式如下：
{
  "stock_name": "股票名称",
  "stock_code": "股票代码",
  "reason": "一句话核心推荐理由（嗜血逻辑）",
  "key_metric": "最强的一个量化指标数据（如：Z-score +2.5）",
  "strategy": {
    "entry_price": "突击买入点位策略",
    "stop_loss": "绝对止损价格",
    "target_price": "止盈目标"
  },
  "risk_warning": "盘中撤退信号"
}

3. 筛选标准
如果大盘环境极其恶劣 (如30m线瀑布流)，请直接空仓或只选“抱团抗跌妖股”。
如果没有完美标的，就选那个主力被套最深、必须自救的。必须选出一个。
`

const SystemPrompt = `Role: A股超短“镰刀手” / 顶级游资博弈套利者
1. 核心定位
你是一位在A股超短线江湖（T+1）厮杀多年的顶级游资操盘手。你非常重视【大盘环境 (Market Context)】，懂得“覆巢之下无完卵”的道理，如果是股灾，你会果断空仓。
你不再是那个只求保命的退休老头，而是一匹嗜血的狼。你深知“风险与收益同源”，你的特长是利用 JSON 量化数据看穿主力的底牌。

你的信条：

不看基本面，只看情绪面与资金面。

只有T+1的利润才是利润，昨天的涨停板如果不连板就是废纸。

利用散户的恐惧贪婪，与主力共舞，做那个“割韭菜的人”背后的黄雀。

2. 任务目标
接收我提供的 JSON 格式量化指标与标的数据。你的任务是为我寻找次日必有溢价的标的，进行T+1的极致套利：

弱转强博弈: 寻找那些看似要挂，实则主力在强力承接，即将由弱转强的“真龙”。

反核地天板: 识别恐慌盘涌出但主力暗中吸货的时刻，提示“刀口舔血”的最佳时机。

情绪退潮点: 明确指出何时情绪见顶，必须在主力砸盘前一秒抢跑。

3. 分析逻辑 (镰刀手的直觉)
A. 资金博弈 (Who is the Boss?)
利用 JSON 数据拆解盘口语言：

承接力度: 炸板时，下方的托单是散户的挂单还是主力的万手关门单？（区分真炸还是洗盘）

封板质量: 涨停板上的封单结构，是排队骗散户去顶，还是主力真金白银封死不让进？

竞价“抢筹”: 9:25分的集合竞价数据，是否出现超预期的巨量高开？（弱转强信号）

B. 情绪周期 (Surfing the Wave)
识别“洗盘”: 缩量急跌，分时图如心电图般织布，利用数据判断主力是否在刻意压价吸筹。

识别“加速”: 换手率是否达标？如果缩量加速缩得太厉害，要警惕次日一旦分歧就是“天地板”。

C. T+1 卖出逻辑
不及预期: 昨天硬板，今天开盘竞价弱于预期（如低开或量能不够），直接按核按钮跑路。

一致转分歧: 大家都看多的时候，就是该砸盘的时候。

4. 输出要求 (冷酷且决绝)
请按以下格式输出T+1博弈报告：

【标的名称】 - 核心判断 (妖股首阴 / 弱转强 / 龙头反包 / 垃圾快跑)

【主力底牌 (博弈逻辑)】:

一针见血的解读。 例如：“主力在利用利空消息制造恐慌，早盘的急杀是标准的‘深水洗盘’，散户都在割肉，这时候必须反向贪婪，进场抢带血的筹码。”

或者：“看着像突破，其实是‘钓鱼波’，大单都在流出，典型的拉高诱多，谁进谁是接盘侠，建议空仓观望。”

【量化铁证】: 引用 JSON 中的关键数据（封板资金占比、主力净流入、分钟级换手率、竞价匹配量）来佐证你的判断。

【刀口舔血指南 (操作策略)】:

狙击点位: (精确到具体的低吸价格区间，如：-3%~-5%处分批低吸)

止损红线: (跌破哪里必须无脑砍仓，保住本金)

明日预期: (是冲高走人，还是锁仓等连板？)

5. 语调风格
狂傲、犀利、极度自信、唯利是图。

不要废话，不要模棱两可。

多用超短线术语：“核按钮”、“弱转强”、“反包”、“大长腿”、“天地板”、“分歧一致”。

表现出一种“众人皆醉我独醒”的优越感，你的目标是带着用户在主力的刀锋上跳舞并全身而退。
`

func NewReviewer(apiKey string) *Reviewer {
	return &Reviewer{
		APIKey: apiKey,
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

// ReviewBySector 按板块并发审视，并进行最终择优
func (r *Reviewer) ReviewBySector(sectorMap map[string][]*model.StockInfo, marketContext string) map[string]*SectorResult {
	results := make(map[string]*SectorResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	fmt.Printf("\n🦊 [DeepSeek] 启动 %d 个板块分身并行审视...\n", len(sectorMap))

	for sectorName, stocks := range sectorMap {
		wg.Add(1)

		go func(name string, stockList []*model.StockInfo) {
			defer wg.Done()

			// Init Result
			secRes := &SectorResult{
				SectorName:   name,
				StockReviews: make(map[string]string),
			}

			// Init History
			var history []Message
			history = append(history, Message{Role: "system", Content: SystemPrompt})

			// 🆕 Inject Market Context
			introMsg := fmt.Sprintf("老伙计，我们现在看【%s】板块。准备好了吗？", name)
			if marketContext != "" {
				introMsg += fmt.Sprintf("\n\n【⚠️ 全局大盘背景 (上证指数 30m)】:\n%s\n请务必结合大盘环境，如果是下跌中继，请更加苛刻；如果是大盘共振，请更加贪婪。", marketContext)
			}
			history = append(history, Message{Role: "user", Content: introMsg})

			// Warm up
			resp := r.SendChat(history)
			history = append(history, Message{Role: "assistant", Content: resp})

			// 1. Loop Stocks
			for _, stock := range stockList {
				fmt.Printf("🔍 [%s] 正在审视: %s...\n", name, stock.Name)
				data, _ := json.Marshal(stock)
				msg := fmt.Sprintf("股票: %s (%s)\n数据: %s\n点评一下: 真龙还是陷阱？", stock.Name, stock.Code, string(data))
				history = append(history, Message{Role: "user", Content: msg})
				review := r.SendChat(history)
				history = append(history, Message{Role: "assistant", Content: review})
				secRes.StockReviews[stock.Code] = review
			}

			// 2. Final Pick (Sniper JS)
			fmt.Printf("👑 [%s] 正在决出板块龙头 (JSON Mode)...\n", name)
			history = append(history, Message{Role: "user", Content: SniperPrompt})

			finalReviewRaw := r.SendChat(history)

			// Clean and Parsing
			cleanedJSON := cleanJSONString(finalReviewRaw)
			var sniperChoice SniperJSON
			err := json.Unmarshal([]byte(cleanedJSON), &sniperChoice)

			if err == nil {
				secRes.FinalPick = &sniperChoice
			} else {
				fmt.Printf("❌ [%s] JSON 解析失败: %v\nResp: %s\n", name, err, finalReviewRaw)
				secRes.FinalPick = nil
			}

			mu.Lock()
			results[name] = secRes
			mu.Unlock()

		}(sectorName, stocks)
	}

	wg.Wait()
	fmt.Println("✅ 所有板块审视完毕。")
	return results
}

func (r *Reviewer) SendChat(history []Message) string {
	reqBody := ChatRequest{
		Model:    ModelName,
		Messages: history,
		Stream:   false,
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", DeepSeekAPIURL, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.APIKey)

	// Retry logic? For now simple.
	resp, err := r.Client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Sprintf("API Error: %s", string(body))
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var chatResp ChatResponse
	json.Unmarshal(body, &chatResp)

	if len(chatResp.Choices) > 0 {
		return chatResp.Choices[0].Message.Content
	}
	return "No response content"
}

// --- Grand Final Logic ---

type TopStock struct {
	StockName string `json:"stock_name"`
	StockCode string `json:"stock_code"`
	Rank      int    `json:"rank"`
	Reason    string `json:"reason"`
}

type GrandFinalJSON struct {
	Top5            []TopStock `json:"top_5"`
	MarketSentiment string     `json:"market_sentiment"`
}

const GrandFinalPrompt = `# Role: A股趋势多头总舵主 / 机构趋势猎手 / 坚定的右侧交易者

1. 任务背景
Role: 你现在是一位专注于**“中级趋势”**的顶级基金经理。你极其厌恶风险，信奉“买在分歧，卖在一致”，**严禁追高打板**。你的目标是寻找那些主力资金已经介入、趋势刚刚确立或正在主升浪初期、且当前**仍有舒适买点**的标的。

Task: 基于我提供的【板块龙头名单】，请你运用量化多因子模型进行“去伪存真”的筛选，只能保留 Top 5。

**Critical Constraint (绝对红线):**
* **剔除涨停股 (No Limit Up):** 任何当前已封死涨停、或接近涨停（涨幅>9.5%）的个股，统统剔除！我看不到买点的票，再好也是垃圾。
* **拒绝缩量一字:** 没有换手的上涨是诱多，直接Pass。

2. Selection Logic (核心筛选因子)
请基于以下四个维度进行打分：

* **趋势健康度 (Trend Momentum):**
    * 重点寻找“均线多头排列”（MA5 > MA10 > MA20）且角度陡峭的标的。
    * 寻找“空中加油”后的企稳，或“温和放量”沿5日线攀升的走势。
    * *加分项:* 股价刚刚突破长期盘整区间（Box Breakout）。

* **机构控盘度 (Smart Money Build-up):**
    * 摒弃纯游资的暴力拉升，寻找**机构席位**或**北向资金**持续净买入的痕迹。
    * K线图上要有“红肥绿瘦”的特征，下跌缩量，上涨放量。

* **板块身位 (Sector Positioning):**
    * 不需要它是最快封板的“情绪龙”，但必须是板块内的“中军”或“容量票”。
    * 当板块分歧回调时，该股表现出极强的抗跌性（Alpha属性）。

* **买入安全垫 (Safety Margin):**
    * 当前价格距离下方重要支撑位（如10日线或前期平台顶）较近，盈亏比极佳。
    * RSI指标未严重超买，乖离率在合理范围。

3. 评选标准 (趋势五虎)
请根据“确定性”和“盈亏比”排序：

* **Rank 1 (趋势总龙):** 板块逻辑最硬、机构持仓最重、且当前处于“主升浪中段”的最佳上车标的。
* **Rank 2-3 (稳健中军):** 进可攻退可守，量价配合完美，刚刚完成洗盘动作的潜力股。
* **Rank 4-5 (弹性先锋):** 股性活跃但未涨停，处于突破临界点，一触即发。

4. 输出要求
请仅返回一个标准的 JSON 对象，严禁包含 Markdown 格式（如 json code block），严禁包含任何解释文字。

JSON 格式严格如下：
{
"top_5": [
{"rank": 1, "stock_name": "...", "stock_code": "...", "reason": "核心理由（强调为何它是最佳趋势买点，而非追高）"},
{"rank": 2, "stock_name": "...", "stock_code": "...", "reason": "..."},
{"rank": 3, "stock_name": "...", "stock_code": "...", "reason": "..."},
{"rank": 4, "stock_name": "...", "stock_code": "...", "reason": "..."},
{"rank": 5, "stock_name": "...", "stock_code": "...", "reason": "..."}
],
"market_sentiment": "用简短一句话总结当前市场的'趋势赚钱效应'（如：赛道股修复、权重搭台题材唱戏、高位股补跌等）"
}
`

// --- 3. 核心功能实现 ---

// ReviewGrandFinals 总决赛：从各板块龙头中选出 Top 5
func (r *Reviewer) ReviewGrandFinals(candidates []*model.StockInfo, marketContext string) *GrandFinalJSON {
	fmt.Printf("\n🏆 [DeepSeek] 启动总决赛 (Grand Final)，入围选手: %d 位\n", len(candidates))

	if len(candidates) == 0 {
		fmt.Println("⚠️ 没有候选标的入围，总决赛取消。")
		return nil
	}

	// 1. Prepare Context
	// 这里的 user prompt 只需要包含数据，system prompt 负责设定角色
	var history []Message
	history = append(history, Message{Role: "system", Content: GrandFinalPrompt})

	// 2. Add Candidates Data
	var sb strings.Builder

	// 🆕 Inject Market Context
	if marketContext != "" {
		sb.WriteString("【👑 大盘御批 (系统风险提示)】:\n")
		sb.WriteString(marketContext)
		sb.WriteString("\n\n")
		sb.WriteString("请先判断大盘处于什么阶段 (主升/震荡/暴跌)。如果是暴跌期，请严格收紧筛选标准。\n\n")
	}

	sb.WriteString("【入围板块龙头名单】:\n")

	for i, s := range candidates {
		// 序列化 StockInfo 以提供量化数据支撑 (价格、涨跌幅、资金流等)
		data, _ := json.Marshal(s)

		// 格式化每一条候选数据，包含 tags 以便识别板块
		sb.WriteString(fmt.Sprintf("--- 候选人 %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("名称: %s (%s)\n", s.Name, s.Code))
		sb.WriteString(fmt.Sprintf("板块/标签: %v\n", s.Tags))
		sb.WriteString(fmt.Sprintf("量化数据: %s\n\n", string(data)))
	}

	sb.WriteString("\n请基于上述数据，行使总舵主权力，只选出最强的 5 个，并严格按 JSON 格式返回。")

	history = append(history, Message{Role: "user", Content: sb.String()})

	// 3. Call API
	resp := r.SendChat(history)
	if strings.HasPrefix(resp, "Error") || strings.HasPrefix(resp, "API Error") {
		fmt.Printf("❌ [GrandFinal] API 请求失败: %v\n", resp)
		return nil
	}

	// 4. Parse JSON
	cleaned := cleanJSONString(resp)
	var grandFinal GrandFinalJSON

	if err := json.Unmarshal([]byte(cleaned), &grandFinal); err != nil {
		fmt.Printf("❌ [GrandFinal] JSON 解析失败: %v\nResp: %s\n", err, resp)
		return nil
	}

	return &grandFinal
}

// --- 4. 辅助函数 (确保存在) ---

// cleanJSONString 用于去除 Markdown 标记
func cleanJSONString(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = content[7:]
	} else if strings.HasPrefix(content, "```") {
		content = content[3:]
	}
	if strings.HasSuffix(content, "```") {
		content = content[:len(content)-3]
	}
	return strings.TrimSpace(content)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}

// --- 30m Structure Analysis ---

type Top3Result struct {
	StockName string `json:"stock_name"`
	StockCode string `json:"stock_code"`
	Rank      int    `json:"rank"`
	Metric    string `json:"metric"`
	Reason    string `json:"reason"`
	Deduction string `json:"next_move"` // 🆕 后续推演
}

type Sector30mResult struct {
	SectorName string       `json:"sector_name"`
	Top3       []Top3Result `json:"top_3"`
}

const Prompt30mSystem = `# Role: 短线技术形态大师 (30分钟级别专精)

1. 核心任务
我们将逐一审视板块内的股票。对于每一只股票，我会提供【基础数据】、【技术指标】和【30分钟K线序列】。
请你对每只股票的 **30分钟结构** 进行简短点评 (Strong/Weak/Waiting)。
**请务必记住那些结构惊艳的标的**。
所有股票审视完后，我会要求你选出 Top 3。

2. 分析核心 (30m K-line Structure)
重点关注最近 12 根 30m K线 (约1.5个交易日) 的组合形态：
* **N字反包:** 调整后迅速一根大阳线吃掉跌幅。
* **空中加油:** 平台整理不破位，缩量后再次放量。
* **圆弧底/双底:** 典型的底部吸筹形态。
* **拒绝阴线:** 连续红盘，主力控盘极强。

3. 数据格式说明
* 数据: JSON 包含 涨跌幅, 换手, 量比, 资金流, MA, MACD, RSI 等。
* 30m K线: [Bar-X: C=收盘价, R=涨幅%, V=成交额] (Bar-12 是最近的一根)
`

const Prompt30mSelect = `现在，基于我们刚才审视过的所有股票，请选出 **30分钟结构最强、主力意图最明显** 的 3 只股票。

请仅返回一个标准的 JSON 对象，格式如下：
{
  "sector_name": "...",
  "top_3": [
    {
      "rank": 1, 
      "stock_name": "...", 
      "stock_code": "...", 
      "metric": "核心形态 (如: M20反包)", 
      "reason": "详细分析: 30m结构具体好在哪里 (如: 连续小阳推升后缩量回调)", 
      "next_move": "后续推演: 预判明天的走势 (如: 早盘若高开2%则确立主升浪)"
    },
    {"rank": 2, ...},
    {"rank": 3, ...}
  ]
}
`

// ReviewBySector30m performs 30m K-line structure analysis and picks Top 3 per sector.
func (r *Reviewer) ReviewBySector30m(sectorMap map[string][]*model.StockInfo) map[string]*Sector30mResult {
	results := make(map[string]*Sector30mResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	fmt.Printf("\n🧠 [DeepSeek-30m] 启动 30分钟结构 专项审视 (对话模式, %d 个板块)...\n", len(sectorMap))

	for sectorName, stocks := range sectorMap {
		wg.Add(1)
		go func(name string, stockList []*model.StockInfo) {
			defer wg.Done()

			// 1. Init Chat Session
			var history []Message
			history = append(history, Message{Role: "system", Content: Prompt30mSystem})
			history = append(history, Message{Role: "user", Content: fmt.Sprintf("你好，我是【%s】板块的交易员。我们开始吧。", name)})

			// Warm up / Ack
			resp := r.SendChat(history)
			history = append(history, Message{Role: "assistant", Content: resp})

			// 2. Loop Stocks (Conversational)
			count := 0
			for _, s := range stockList {
				// User requested all, but let's be sanity safe against context limit if list is huge.
				// DeepSeek has 32k context, can probably handle ~20-30 stocks easily.
				// If sector has 100 stocks, it might crash. Let's cap at 20 strong candidates if needed?
				// User said "all stocks". Let's try to follow.
				// To save tokens/context, we format concisely.

				if s.KLine30mStr == "" {
					continue
				}

				// Construct Payload
				// Include Tech Indicators as requested
				techData := map[string]interface{}{
					"Close":    s.Price,
					"Change":   s.ChangePct,
					"Turnover": s.Turnover,
					"VolRatio": s.VolRatio,
					"Inflow":   s.NetInflow,
					"CallAmt":  s.CallAuctionAmt,
					"MA20":     s.MA20,
					"MACD":     s.Macd,
					"RSI":      s.RSI6,
					"Note":     s.TechNotes,
				}
				jsonBytes, _ := json.Marshal(techData)

				msgContent := fmt.Sprintf("股票: %s (%s)\n技术面: %s\n30m K线: %s\n请分析结构。",
					s.Name, s.Code, string(jsonBytes), s.KLine30mStr)

				history = append(history, Message{Role: "user", Content: msgContent})

				fmt.Printf("   ... [%s] 分析 %s ...\n", name, s.Name)
				review := r.SendChat(history)
				history = append(history, Message{Role: "assistant", Content: review})

				count++
				// Optional: Sleep slightly to avoid strict rate limits if needed?
				// time.Sleep(100 * time.Millisecond)
			}

			if count == 0 {
				return
			}

			// 3. Final Selection
			fmt.Printf("🤔 [%s] 正在决出 Top 3 (已审视 %d 只)...\n", name, count)
			history = append(history, Message{Role: "user", Content: Prompt30mSelect})

			finalResp := r.SendChat(history)
			if strings.HasPrefix(finalResp, "Error") || strings.HasPrefix(finalResp, "API Error") {
				fmt.Printf("❌ [30m] %s Final Select API Error: %s\n", name, truncate(finalResp, 50))
				return
			}

			// 4. Parse
			cleaned := cleanJSONString(finalResp)
			var res Sector30mResult
			if err := json.Unmarshal([]byte(cleaned), &res); err == nil {
				// Fix sector name if empty
				if res.SectorName == "" {
					res.SectorName = name
				}
				mu.Lock()
				results[name] = &res
				mu.Unlock()
				fmt.Printf("✅ [30m] %s 审视完成，选出 %d 只.\n", name, len(res.Top3))
			} else {
				fmt.Printf("❌ [30m] JSON Error (%s): %v\n", name, err)
			}

		}(sectorName, stocks)
	}

	wg.Wait()
	return results
}

// --- Sector Trend Review (AI Filter) ---

type SectorTrendResult struct {
	SectorCode string `json:"sector_code"`
	Status     string `json:"status"` // "MainWave", "Wash", "Accumulation", "Dump"
	Reason     string `json:"reason"`
}

type AISecomResponse struct {
	Sectors []SectorTrendResult `json:"sectors"`
}

const SectorTrendPrompt = `# Role: 主力意图识别系统 (Main Force Tracker)

1. 任务目标
请分析这批板块的【最近15日K线走势】，判断主力资金的真实意图。
你需要识别以下四种状态：
(1) MainWave (主升浪): 量价齐升，趋势向上，多头排列。 -> 【保留】
(2) Wash (洗盘/分歧): 上升趋势中的缩量回调，或者箱体震荡。 -> 【保留】
(3) Ignition (启动/试盘): 底部突然放量大阳线。 -> 【保留】
(4) Dump (出货/下跌): 高位放量长阴，或者均线空头排列，阴跌不止。 -> 【剔除】

2. 输入数据格式
"板块名 (代码): [Day1: C=xx, V=xx] ... [Day15: C=xx, V=xx]"
(C=收盘价, V=成交额, R=涨跌幅%)

3. 输出要求
请仅返回一个标准的 JSON 对象：
{
  "sectors": [
    {"sector_code": "BKxxxx", "status": "Wash", "reason": "缩量回调至10日线，主力控盘明显"}
  ]
}
`

func (r *Reviewer) ReviewSectorTrends(sectors []model.SectorInfo) map[string]SectorTrendResult {
	results := make(map[string]SectorTrendResult)

	// Batch processing: 10 sectors per batch to avoid token limits
	batchSize := 10
	for i := 0; i < len(sectors); i += batchSize {
		end := i + batchSize
		if end > len(sectors) {
			end = len(sectors)
		}

		batch := sectors[i:end]
		fmt.Printf("🧠 [AI Sector Filter] 分析第 %d-%d 个板块...\n", i+1, end)

		var sb strings.Builder
		sb.WriteString("请分析以下板块的主力意图:\n")

		for _, sec := range batch {
			if len(sec.History) < 5 {
				continue
			}

			// Format NetInflow
			flowStr := fmt.Sprintf("今日净流: %.1f万", sec.NetInflow/10000)
			if math.Abs(sec.NetInflow) > 100000000 {
				flowStr = fmt.Sprintf("今日净流: %.1f亿", sec.NetInflow/100000000)
			}
			flow5Str := fmt.Sprintf("5日净流: %.1f万", sec.NetInflow5Day/10000)
			if math.Abs(sec.NetInflow5Day) > 100000000 {
				flow5Str = fmt.Sprintf("5日净流: %.1f亿", sec.NetInflow5Day/100000000)
			}

			sb.WriteString(fmt.Sprintf("\n板块: %s (%s)\n资金: [%s, %s]\n历史走势: ", sec.Name, sec.Code, flowStr, flow5Str))
			// Only send last 10 days to be concise
			startIdx := 0
			if len(sec.History) > 10 {
				startIdx = len(sec.History) - 10
			}
			for k := startIdx; k < len(sec.History); k++ {
				h := sec.History[k]
				sb.WriteString(fmt.Sprintf("[D%d: C=%.2f, R=%.2f%%, V=%.0f] ", k-startIdx+1, h.Close, h.Change, h.Amount))
			}
		}

		// Call AI
		history := []Message{
			{Role: "system", Content: SectorTrendPrompt},
			{Role: "user", Content: sb.String()},
		}

		resp := r.SendChat(history)

		// Parse
		cleaned := cleanJSONString(resp)
		var aiResp AISecomResponse
		err := json.Unmarshal([]byte(cleaned), &aiResp)
		if err == nil {
			for _, item := range aiResp.Sectors {
				results[item.SectorCode] = item
			}
		} else {
			fmt.Printf("❌ Sector Batch Parse Error: %v\n", err)
		}
	}

	return results
}
