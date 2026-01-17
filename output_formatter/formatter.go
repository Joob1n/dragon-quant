package output_formatter

import (
	"dragon-quant/config"
	"dragon-quant/model"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"
)

// --- Color Constants ---
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorBold   = "\033[1m"
)

func GenFiles(cfg *config.Config, allSectors []model.SectorInfo, stocks []*model.StockInfo, elapsed time.Duration, sentiment string) {
	aiReport := model.AIReport{}
	aiReport.Meta.ScanTime = cfg.StartTsStr
	aiReport.Meta.Version = "v10.1 Dragon Sniper (Sentiment+Sustainability)"
	aiReport.Meta.Desc = "Fields: CallAuction(f277), LHB, ProfitDev, DragonHabit, OpenVolRatio, Sentiment"
	aiReport.Stats.Sentiment = sentiment

	htmlReport := model.ReportData{Time: cfg.StartTsStr, Sentiment: sentiment, TotalCount: len(stocks), Duration: elapsed.String()}

	maxInflow := 0.0
	topSector := ""
	dragonCount := 0

	for _, sec := range allSectors {
		var groupStocks []model.StockInfo
		var htmlItems []model.StockItem
		totalChg := 0.0
		totalInflow := 0.0

		for _, s := range stocks {
			if Contains(s.Tags, sec.Name) {
				groupStocks = append(groupStocks, *s)
				totalChg += s.ChangePct
				totalInflow += s.NetInflow
				if s.OpenAmt > 10000000 {
					dragonCount++
				} // 竞价>1000万就算强

				var otherTags []string
				for _, t := range s.Tags {
					if t != sec.Name {
						otherTags = append(otherTags, t)
					}
				}

				inflowStr := fmt.Sprintf("%.1f万", s.NetInflow/10000)
				if math.Abs(s.NetInflow) > 100000000 {
					inflowStr = fmt.Sprintf("%.2f亿", s.NetInflow/100000000)
				}

				inflow3Str := fmt.Sprintf("%.1f万", s.NetInflow3Day/10000)
				if math.Abs(s.NetInflow3Day) > 100000000 {
					inflow3Str = fmt.Sprintf("%.2f亿", s.NetInflow3Day/100000000)
				}

				inflow5Str := fmt.Sprintf("%.1f万", s.NetInflow5Day/10000)
				if math.Abs(s.NetInflow5Day) > 100000000 {
					inflow5Str = fmt.Sprintf("%.2f亿", s.NetInflow5Day/100000000)
				}

				callStr := fmt.Sprintf("%.1f万", s.CallAuctionAmt/10000)
				if s.CallAuctionAmt > 100000000 {
					callStr = fmt.Sprintf("%.2f亿", s.CallAuctionAmt/100000000)
				}

				// 龙虎榜显示
				lhbDisplay := "-"
				if s.LHBInfo != "" {
					lhbDisplay = s.LHBInfo
				}

				// 买一显示
				buy1Str := fmt.Sprintf("%d手", s.Buy1Vol)

				// 承接率显示
				openRatioStr := fmt.Sprintf("%.2f", s.OpenVolRatio)
				if s.OpenVolRatio < 0.5 {
					openRatioStr = fmt.Sprintf("⚠️ %.2f (弱承接)", s.OpenVolRatio)
				} else if s.OpenVolRatio > 2.0 {
					openRatioStr = fmt.Sprintf("🔥 %.2f (强承接)", s.OpenVolRatio)
				}

				htmlItems = append(htmlItems, model.StockItem{
					Code: s.Code, Name: s.Name, Price: fmt.Sprintf("%.2f", s.Price),
					Change:       fmt.Sprintf("%+.2f%%", s.ChangePct),
					Turnover:     fmt.Sprintf("%.1f%%", s.Turnover),
					VolRatio:     fmt.Sprintf("%.1f", s.VolRatio),
					Amplitude:    fmt.Sprintf("%.1f%%", s.Amplitude),
					Inflow:       inflowStr,
					Inflow3:      inflow3Str,
					Inflow5:      inflow5Str,
					CallAuctions: callStr,
					LHBStr:       lhbDisplay,
					Buy1Str:      buy1Str,
					ProfitDev:    fmt.Sprintf("%.1f%%", s.ProfitDev*100),
					OpenVolRatio: openRatioStr,
					Habit:        s.DragonHabit,
					Status:       s.DragonTag,
					Tech:         s.TechNotes,
					Note30m:      s.Note30m,
					Tags:         strings.Join(otherTags, " "),
					IsLimitUp:    s.ChangePct > 9.5,
				})
			}
		}

		if len(groupStocks) > 0 {
			if totalInflow > maxInflow {
				maxInflow = totalInflow
				topSector = sec.Name
			}
			avg := totalChg / float64(len(groupStocks))

			secFlowStr := fmt.Sprintf("%.1f万", totalInflow/10000)
			if math.Abs(totalInflow) > 100000000 {
				secFlowStr = fmt.Sprintf("%.2f亿", totalInflow/100000000)
			}

			var readableStocks []model.ReadableStockInfo
			for _, s := range groupStocks {
				readableStocks = append(readableStocks, model.ReadableStockInfo{
					Code:           s.Code,
					Name:           s.Name,
					Price:          s.Price,
					ChangePct:      s.ChangePct,
					Turnover:       s.Turnover,
					VolRatio:       s.VolRatio,
					NetInflow:      s.NetInflow,
					NetInflow3Day:  s.NetInflow3Day,
					NetInflow5Day:  s.NetInflow5Day,
					Amplitude:      s.Amplitude,
					OpenAmt:        s.OpenAmt,
					Tags:           s.Tags,
					CallAuctionAmt: s.CallAuctionAmt,
					LHBInfo:        s.LHBInfo,
					LHBNet:         s.LHBNet,
					Buy1Vol:        s.Buy1Vol,
					Buy1Price:      s.Buy1Price,
					Sell1Vol:       s.Sell1Vol,
					VWAP:           s.VWAP,
					ProfitDev:      s.ProfitDev,
					OpenVolRatio:   s.OpenVolRatio,
					DragonHabit:    s.DragonHabit,
					BoardCount:     s.BoardCount,
					DragonTag:      s.DragonTag,
					MA5:            s.MA5,
					MA20:           s.MA20,
					DIF:            s.DIF,
					DEA:            s.DEA,
					Macd:           s.Macd,
					RSI6:           s.RSI6,
					TechNotes:      s.TechNotes,
				})
			}

			aiReport.Sectors = append(aiReport.Sectors, model.SectorAnalysis{
				Name: sec.Name, Type: sec.Type, Count: len(groupStocks), AvgChange: avg, NetInflow: totalInflow, Stocks: readableStocks,
			})

			sort.Slice(htmlItems, func(i, j int) bool {
				// Parse back for sorting if needed, or better just use original slice order logic
				// But here we rely on the fact 'groupStocks' was NOT sorted yet, 'stocks' was sorted by CallAuctionAmt
				// So we should respect input order or re-sort.
				// Let's re-sort by CallAuction (need to access raw data or parse string... simplest is trust input order or Parse)
				return false // Simplified for now, relies on main sort
			})
			htmlReport.Groups = append(htmlReport.Groups, model.SectorGroup{
				Type: sec.Type, Name: sec.Name, Count: len(groupStocks), AvgChange: fmt.Sprintf("%.2f%%", avg), AvgInflow: secFlowStr, Stocks: htmlItems,
			})
		}
	}

	aiReport.Stats.TopSector = topSector
	aiReport.Stats.DragonCount = dragonCount

	// Sort sectors by NetInflow
	sort.Slice(aiReport.Sectors, func(i, j int) bool { return aiReport.Sectors[i].NetInflow > aiReport.Sectors[j].NetInflow })

	jsonBytes, _ := json.MarshalIndent(aiReport, "", "  ")
	ioutil.WriteFile(cfg.JsonFile, jsonBytes, 0644)
	GenerateHTML(cfg.DragonReportFile, htmlReport)

	fmt.Printf("\n📄 [手机战报] DragonReport_%s.html\n", cfg.StartTsStr)
	fmt.Printf("🤖 [AI 数据] AI_Dragon_%s.json\n", cfg.StartTsStr)
}

func GenerateHTML(filename string, data model.ReportData) {
	const tpl = `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>天启狙击战报</title>
<style>
body{background:#0d1117;color:#c9d1d9;font-family:-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,sans-serif;margin:0;padding:12px}
.header{border-bottom:1px solid #30363d;padding-bottom:12px;margin-bottom:16px}
.title{font-size:20px;font-weight:700;color:#f85149;letter-spacing:1px}
.meta{font-size:11px;color:#8b949e;margin-top:6px}
.sector{background:#161b22;border-radius:8px;margin-bottom:16px;overflow:hidden;border:1px solid #30363d}
.sec-head{background:#21262d;padding:10px 14px;display:flex;justify-content:space-between;align-items:center}
.sec-name{color:#ffd43b;font-weight:600;font-size:15px}
.sec-stat{font-size:12px;color:#8b949e}
table{width:100%;border-collapse:collapse;font-size:13px}
td{padding:12px 10px;border-bottom:1px solid #21262d;vertical-align:top}
.c-name{width:38%} .c-data{width:32%;text-align:right} .c-call{width:30%;text-align:right}
.name{font-weight:600;font-size:14px;color:#e6edf3;display:block;margin-bottom:2px}
.code{font-size:11px;color:#8b949e}
.status-tag{font-size:9px;padding:1px 4px;border-radius:2px;margin-left:4px;vertical-align:text-bottom}
.status-1{background:#238636;color:#fff} .status-2{background:#1f6feb;color:#fff} .status-3{background:#a371f7;color:#fff}
.tag{display:inline-block;font-size:9px;background:#30363d;color:#8b949e;padding:1px 4px;border-radius:3px;margin-top:4px}
.price{font-family:monospace;font-size:14px}
.change{font-family:monospace;font-weight:700;font-size:14px;margin-top:2px;display:block}
.sub-data{font-size:10px;color:#8b949e;margin-top:4px;display:block}
.call{font-family:monospace;font-weight:700;font-size:13px;color:#e2e8f0}
.flow{font-size:10px;color:#ff7b72;margin-top:2px;display:block}
.up{color:#ff7b72} .limit{color:#ff7b72;text-shadow:0 0 8px rgba(255,123,114,0.4)}
.call-high{color:#f85149}
</style>
</head>
<body>
<div class="header">
 <div class="title">🐲 天启·龙头狙击 v10.1</div>
 <div class="meta">策略: 竞价抢筹 + 连板推演 + 资金流 + 情绪监控</div>
 <div class="meta">生成: {{.Time}} | 情绪: <span style="color:#f85149;font-weight:bold">{{.Sentiment}}</span> | 命中: {{.TotalCount}}只</div>
</div>
{{range .Groups}}
<div class="sector">
 <div class="sec-head">
  <span class="sec-name">{{.Name}}</span>
  <span class="sec-stat">净流 <span class="up">{{.AvgInflow}}</span></span>
 </div>
 <table>
  {{range .Stocks}}
  <tr>
   <td class="c-name">
    <span class="name">{{.Name}} 
     {{if eq .Status "3连板+"}}<span class="status-tag status-3">3板+</span>{{else if eq .Status "2连板"}}<span class="status-tag status-2">2板</span>{{end}}
    </span>
    <span class="code">{{.Code}}</span>
    <span class="sub-data" style="color:#d2a8ff">{{.LHBStr}}</span>
    {{if .Tags}}<div class="tag">{{.Tags}}</div>{{end}}
   </td>
   <td class="c-data">
    <span class="price">{{.Price}}</span>
    <span class="change {{if .IsLimitUp}}limit{{else}}up{{end}}">{{.Change}}</span>
    <span class="sub-data">量比{{.VolRatio}} | 振{{.Amplitude}}</span>
    <span class="sub-data">承接: {{.OpenVolRatio}}</span>
   </td>
   <td class="c-call">
    <div class="call">{{.CallAuctions}}</div>
    <span class="flow">净:{{.Inflow}}</span>
    <span class="sub-data">3日:{{.Inflow3}} | 5日:{{.Inflow5}}</span>
    <span class="flow">获利盘: {{.ProfitDev}}</span>
    <span class="sub-data">股性: {{.Habit}}</span>
    <span class="sub-data" style="color:#d2a8ff">{{.LHBStr}}</span>
    <span class="sub-data">{{.Tech}}</span>
    <span class="sub-data" style="color:#a371f7">30m: {{.Note30m}}</span>
   </td>
  </tr>
  {{end}}
 </table>
</div>
{{end}}
<div style="text-align:center;color:#444;font-size:10px;margin-top:30px">Dragon Sniper v10.0 (Memory Core)</div>
</body></html>`
	t, _ := template.New("report").Parse(tpl)
	f, _ := os.Create(filename)
	defer f.Close()
	t.Execute(f, data)
}

func PrintBanner() {
	fmt.Println(ColorRed + `
   ___  ____    _    ____  ____  _   _ 
  / _ \|  _ \  / \  / ___|/ _ \| \ | |
 | | | | |_) |/ _ \| |  _| | | |  \| |
 | |_| |  _ <| ___ | |_| | |_| | |\  |
  \___/|_| \_/_/   \_\____|\___/|_| \_| v10.0
   Apocalypse: Memory + VWAP + LHB
` + ColorReset)
}

func PrintDragonTable(stocks []*model.StockInfo) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "代码\t名称\t地位\t涨幅%\t真实竞价\t无论\t获利盘\t股性\t技术备注\t30m意图")
	fmt.Fprintln(w, "----\t----\t----\t-----\t--------\t------\t------\t----\t--------\t-------")
	for i, s := range stocks {
		if i >= 30 {
			break
		}
		pctStr := fmt.Sprintf("%+.2f%%", s.ChangePct)
		if s.ChangePct > 9.0 {
			pctStr = ColorRed + ColorBold + pctStr + ColorReset
		}

		callStr := fmt.Sprintf("%.0f万", s.CallAuctionAmt/10000)
		if s.CallAuctionAmt > 100000000 {
			callStr = ColorYellow + fmt.Sprintf("%.2f亿", s.CallAuctionAmt/100000000) + ColorReset
		}

		lhbStr := "-"
		if s.LHBInfo != "" {
			lhbStr = ColorPurple + "有榜" + ColorReset
		}

		profitStr := fmt.Sprintf("%.0f%%", s.ProfitDev*100)
		if s.ProfitDev > 0.3 {
			profitStr = ColorRed + profitStr + ColorReset
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Code, s.Name, s.DragonTag, pctStr, callStr, lhbStr, profitStr, s.DragonHabit, s.TechNotes, s.Note30m)
	}
	w.Flush()
}

func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func WriteMD(filename, content string) {
	fmt.Printf("%s", filename)
	ioutil.WriteFile(filename, []byte(content), 0644)
}

func SimpleMDToHTMLFile(mdFile, htmlFile string) {
	content, err := ioutil.ReadFile(mdFile)
	if err != nil {
		fmt.Printf("❌ Read MD Error: %v\n", err)
		return
	}
	html := SimpleMDToHTML(string(content))
	ioutil.WriteFile(htmlFile, []byte(html), 0644)
}
