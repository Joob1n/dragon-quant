# Dragon Quant (龙量化)

AI-driven quantitative trading strategy and research tool.

## Features
- **Apocalypse Strategy**: Multi-factor core including Memory, VWAP, and LHB.
- **DeepSeek Integration**: AI-powered stock review and sector analysis.
- **Risk Control**: "Old Fox" risk screening system.

## Setup
```bash
go build -o dragon-quant
./dragon-quant
```


## Module
Previously known as `awesomeProject33`, now renamed to `dragon-quant`.

## 🛡️ Hold Kline Analysis (持仓深度审视)
A specialized module to analyze 30-minute K-line structures for specific stocks using DeepSeek AI.

### Usage
1. **Configure Stocks**: Open `main.go` and edit the `holdStocks` array with the names of the stocks you want to analyze.
   ```go
   // main.go
   holdStocks := []string{
       "新金路",
       "宏创控股",
       "航天彩虹", 
       "久远银海",
   }
   ```
   *Note: The system automatically searches for the stock code by name.*

2. **Run Analysis**:
   ```bash
   go run main.go -hold-kline
   ```

3. **View Report**:
   Open the generated HTML file, e.g., `Hold_Kline_Report_2026-01-12-23.html`.
