package model

import "encoding/json"

// StockInfo represents the detailed information of a stock.
type StockInfo struct {
	Code          string   `json:"f12"`  // 代码
	Name          string   `json:"f14"`  // 名称
	Price         float64  `json:"f2"`   // 现价
	ChangePct     float64  `json:"f3"`   // 涨跌幅
	Turnover      float64  `json:"f8"`   // 换手率
	VolRatio      float64  `json:"f10"`  // 量比
	NetInflow     float64  `json:"f62"`  // 主力净流入
	NetInflow3Day float64  `json:"f267"` // 3日主力净流入
	NetInflow5Day float64  `json:"f164"` // 5日主力净流入
	Amplitude     float64  `json:"f7"`   // 振幅
	OpenAmt       float64  `json:"f19"`  // 旧:竞价金额(错误) -> 保留兼容
	Tags          []string // 板块标签

	// --- V9.0 新增指标 ---
	CallAuctionAmt float64 `json:"call_auction_amt"` // 真实竞价金额 (f277)
	LHBInfo        string  `json:"lhb_info"`         // 龙虎榜摘要
	LHBNet         float64 `json:"lhb_net"`          // 龙虎榜净买入
	Buy1Vol        int     `json:"buy1_vol"`         // 买一量 (手)
	Buy1Price      float64 `json:"buy1_price"`       // 买一价
	Sell1Vol       int     `json:"sell1_vol"`        // 卖一量 (手)

	// --- V10.0 深度记忆 ---
	VWAP        float64 `json:"vwap"`         // 30日均价
	ProfitDev   float64 `json:"profit_dev"`   // 获利盘乖离率 ((Price-VWAP)/VWAP)
	DragonHabit string  `json:"dragon_habit"` // 股性记忆 (连板/炸板/反包)

	// --- V10.1 高阶指标 ---
	OpenVolRatio float64 `json:"open_vol_ratio"` // 🆕 开盘承接率 (5分成交/竞价成交)

	// --- 衍生指标 ---
	BoardCount int    `json:"board_count"` // 连板高度 (推算)
	DragonTag  string `json:"dragon_tag"`  // 龙头标识 (首板/连板/反包)

	// --- 技术指标 ---
	MA5         float64 `json:"ma5"`
	MA20        float64 `json:"ma20"`
	DIF         float64 `json:"dif"`
	DEA         float64 `json:"dea"`
	Macd        float64 `json:"macd"`
	RSI6        float64 `json:"rsi6"`
	TechNotes   string  `json:"tech_notes"`
	Note30m     string  `json:"note_30m"`      // 30分钟级别分析
	KLine30mStr string  `json:"kline_30m_str"` // 30m K线原始数据
}

type SectorInfo struct {
	Code string `json:"f12"`
	Name string `json:"f14"`
	Type string

	// 🆕 Money Flow
	NetInflow     float64 `json:"f62"`  // 今日主力净流入
	NetInflow5Day float64 `json:"f164"` // 5日主力净流入

	// 🆕 AI Sector Analysis
	History  []KLineData `json:"history"`
	AIView   string      `json:"ai_view"` // "Wash", "MainWave", "Dump"
	AIReason string      `json:"ai_reason"`
}

type KLineData struct {
	Name   string // 🆕 股票/板块名称
	Date   string // 🆕 日期/时间
	Close  float64
	Change float64
	Amount float64 // 成交额
}

// --- API Response ---
type ListResponse struct {
	Data struct {
		Diff []json.RawMessage `json:"diff"`
	} `json:"data"`
}

type KLineResponse struct {
	Data struct {
		Klines []string `json:"klines"`
	} `json:"data"`
}

// --- Report Data ---
type ReportData struct {
	Time       string
	Sentiment  string // 🆕 市场情绪
	TotalCount int
	Duration   string
	Groups     []SectorGroup
}

type SectorGroup struct {
	Type      string
	Name      string
	Count     int
	AvgChange string
	AvgInflow string
	Stocks    []StockItem
}

type StockItem struct {
	Code         string
	Name         string
	Price        string
	Change       string
	Turnover     string
	VolRatio     string
	Inflow       string
	Inflow3      string // 🆕 3日净流
	Inflow5      string // 🆕 5日净流
	CallAuctions string // 🔥 真实竞价金额
	LHBStr       string // 🆕 龙虎榜
	Buy1Str      string // 🆕 买一
	ProfitDev    string // 🆕 获利盘
	OpenVolRatio string // 🆕 承接率 (HTML展示用)
	Habit        string // 🆕 股性
	Status       string // 🔥 龙头地位 (2板/首板)
	Tech         string
	Note30m      string // 🆕 30m意图
	KLine30mStr  string // 🆕 30m K线原始数据 (Prompt用)
	Tags         string
	Amplitude    string
	IsLimitUp    bool
}

// --- AI JSON Data ---
type AIReport struct {
	Meta struct {
		ScanTime string `json:"scan_time"`
		Version  string `json:"version"`
		Desc     string `json:"description"`
	} `json:"meta"`
	Stats struct {
		TopSector   string `json:"top_sector"`
		DragonCount int    `json:"dragon_count"` // 竞价超预期数量
		Sentiment   string `json:"sentiment"`    // 🆕 市场情绪 (昨日涨停表现)
	} `json:"stats"`
	Sectors []SectorAnalysis `json:"sectors"`
}

type SectorAnalysis struct {
	Name      string              `json:"name"`
	Type      string              `json:"type"`
	Count     int                 `json:"count"`
	AvgChange float64             `json:"avg_change"`
	NetInflow float64             `json:"net_inflow"`
	Stocks    []ReadableStockInfo `json:"stocks"`
}

type ReadableStockInfo struct {
	Code          string   `json:"code"`          // 代码 (f12)
	Name          string   `json:"name"`          // 名称 (f14)
	Price         float64  `json:"price"`         // 现价 (f2)
	ChangePct     float64  `json:"change_pct"`    // 涨跌幅 (f3)
	Turnover      float64  `json:"turnover"`      // 换手率 (f8)
	VolRatio      float64  `json:"vol_ratio"`     // 量比 (f10)
	NetInflow     float64  `json:"net_inflow"`    // 主力净流入 (f62)
	NetInflow3Day float64  `json:"net_inflow_3d"` // 3日主力净流入 (f267)
	NetInflow5Day float64  `json:"net_inflow_5d"` // 5日主力净流入 (f164)
	Amplitude     float64  `json:"amplitude"`     // 振幅 (f7)
	OpenAmt       float64  `json:"open_amt"`      // 竞价金额 (f19)
	Tags          []string `json:"tags"`          // 板块标签

	// --- V9.0 新增指标 ---
	CallAuctionAmt float64 `json:"call_auction_amt"` // 真实竞价金额 (f277)
	LHBInfo        string  `json:"lhb_info"`         // 龙虎榜摘要
	LHBNet         float64 `json:"lhb_net"`          // 龙虎榜净买入
	Buy1Vol        int     `json:"buy1_vol"`         // 买一量 (手)
	Buy1Price      float64 `json:"buy1_price"`       // 买一价
	Sell1Vol       int     `json:"sell1_vol"`        // 卖一量 (手)

	// --- V10.0 深度记忆 ---
	VWAP        float64 `json:"vwap"`         // 30日均价
	ProfitDev   float64 `json:"profit_dev"`   // 获利盘乖离率 ((Price-VWAP)/VWAP)
	DragonHabit string  `json:"dragon_habit"` // 股性记忆 (连板/炸板/反包)

	// --- V10.1 高阶指标 ---
	OpenVolRatio float64 `json:"open_vol_ratio"` // 🆕 开盘承接率 (5分成交/竞价成交)

	// --- 衍生指标 ---
	BoardCount int    `json:"board_count"` // 连板高度 (推算)
	DragonTag  string `json:"dragon_tag"`  // 龙头标识 (首板/连板/反包)

	// --- 技术指标 ---
	MA5       float64 `json:"ma5"`
	MA20      float64 `json:"ma20"`
	DIF       float64 `json:"dif"`
	DEA       float64 `json:"dea"`
	Macd      float64 `json:"macd"`
	RSI6      float64 `json:"rsi6"`
	TechNotes string  `json:"tech_notes"`
}

// --- V10.2 二次筛选 (老狐狸逻辑) ---

type RiskConfig struct {
	// 避坑配置
	MaxRSI            float64  `json:"max_rsi"`            // RSI阈值
	MaxProfitDev      float64  `json:"max_profit_dev"`     // 获利盘阈值
	MinVolRatio       float64  `json:"min_vol_ratio"`      // 最小量比
	MaxVolRatio       float64  `json:"max_vol_ratio"`      // 最大量比
	MaxTurnover       float64  `json:"max_turnover"`       // 最大换手率
	MinNetInflow5d    float64  `json:"min_net_inflow_5d"`  // 5日最小净流入
	BlacklistHabits   []string `json:"blacklist_habits"`   // 避开的股性
	BlacklistKeywords []string `json:"blacklist_keywords"` // 技术面避开的关键词

	// 寻宝配置
	GoodHabits    []string `json:"good_habits"`     // 好的股性
	MinBoardCount int      `json:"min_board_count"` // 最小上榜次数
}

type RiskResult struct {
	Stock     *StockInfo
	Reason    string
	RiskScore int // 1-5分
}
