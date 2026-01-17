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
1. **Configure Stocks**: Open `config.yaml` and edit the `hold_stocks` array with the names of the stocks you want to analyze.
   ```yaml
   hold_stocks:
     - "平安银行"
     - "中国中兔"
   ```
   *Note: The system automatically searches for the stock code by name.*

2. **Set DeepSeek Api-Key**: Open `config.yaml` and edit the `deepseek.api_key`, or set the `DS_APIKEY_FOR_DRAGON` in your ENV.
   ```yaml
   deepseek:
     api_key: "your-deepseek-api-key"
   ```
   or
   ```bash
   export DS_APIKEY_FOR_DRAGON='your-deepseek-api-key'
   ```

3. **Run Analysis**:
   ```bash
   go run main.go -hold-kline
   ```

4. **View Report**:
   Open the generated HTML file, e.g., `Hold_Kline_Report_2026-01-12-23.html`.
