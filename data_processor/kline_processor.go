package data_processor

import (
	"database/sql"
	"dragon-quant/model"
	"fmt"
	"time"
)

// AnomalyEvent represents a detected anomaly in the K-line data
type AnomalyEvent struct {
	Time   time.Time `json:"time"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
	AvgVol float64   `json:"roll_avg_vol"`
	StdDev float64   `json:"roll_std_price"`
	Reason string    `json:"reason"`
}

type KlineProcessor struct {
	duck *DuckDB
}

func NewKlineProcessor(d *DuckDB) *KlineProcessor {
	return &KlineProcessor{duck: d}
}

// LoadData loads 1m K-line data into DuckDB kline_1m table
func (p *KlineProcessor) LoadData(klines []model.KLineData) error {
	// 1. Create Table (TIMESTAMP, DOUBLE, DOUBLE)
	_, err := p.duck.DB.Exec(`
		CREATE TABLE IF NOT EXISTS kline_1m (
			time TIMESTAMP, 
			close DOUBLE, 
			volume DOUBLE
		)`)
	if err != nil {
		return fmt.Errorf("create table failed: %w", err)
	}

	// 2. Clear old data (Assuming per-session usage)
	_, err = p.duck.DB.Exec("DELETE FROM kline_1m")
	if err != nil {
		return fmt.Errorf("clear table failed: %w", err)
	}

	// 3. Prepare Insert
	stmt, err := p.duck.DB.Prepare("INSERT INTO kline_1m (time, close, volume) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare insert failed: %w", err)
	}
	defer stmt.Close()

	// 4. Bulk Insert
	// format: "2006-01-02 15:04"
	layout := "2006-01-02 15:04"
	var tx *sql.Tx
	tx, err = p.duck.DB.Begin()
	if err != nil {
		return err
	}

	stmtTx := tx.Stmt(stmt)
	defer stmtTx.Close()

	for _, k := range klines {
		t, err := time.Parse(layout, k.Date)
		if err != nil {
			// Skip or log error? For robustness, skip invalid dates but log?
			// Simplification: try to parse, if fail, skip.
			continue
		}
		_, err = stmtTx.Exec(t, k.Close, k.Amount) // Amount is typically Volume or Turnover. Using Amount here.
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("insert failed: %w", err)
		}
	}

	return tx.Commit()
}

// DetectAnomalies uses Window Functions to find volume/price anomalies
func (p *KlineProcessor) DetectAnomalies() ([]AnomalyEvent, error) {
	// SQL Logic:
	// Calculate Rolling Avg Volume (30 min) and Rolling StdDev Price (30 min)
	// Filter: Volume > 3*Avg OR Abs(Change) > 2*StdDev
	query := `
	WITH stats AS (
		SELECT 
			time, close, volume,
			AVG(volume) OVER (ORDER BY time ROWS BETWEEN 30 PRECEDING AND 1 PRECEDING) as roll_avg_vol,
			STDDEV(close) OVER (ORDER BY time ROWS BETWEEN 30 PRECEDING AND 1 PRECEDING) as roll_std_price,
			LAG(close) OVER (ORDER BY time) as prev_close
		FROM kline_1m
	)
	SELECT time, close, volume, roll_avg_vol, roll_std_price
	FROM stats
	WHERE 
		(volume > 3 * roll_avg_vol AND roll_avg_vol > 0) 
		OR 
		(ABS(close - prev_close) > 2 * roll_std_price AND roll_std_price > 0)
	ORDER BY volume DESC
	LIMIT 5;
	`

	rows, err := p.duck.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("anomaly query failed: %w", err)
	}
	defer rows.Close()

	var events []AnomalyEvent
	for rows.Next() {
		var e AnomalyEvent
		// DuckDB driver handles TIMESTAMP -> time.Time
		if err := rows.Scan(&e.Time, &e.Close, &e.Volume, &e.AvgVol, &e.StdDev); err != nil {
			return nil, err
		}

		reason := ""
		if e.Volume > 3*e.AvgVol {
			reason += fmt.Sprintf("VolSpike(x%.1f) ", e.Volume/e.AvgVol)
		}
		if reason == "" {
			reason = "PriceShock"
		}
		e.Reason = reason
		events = append(events, e)
	}
	return events, nil
}

// GetContextWindow returns data in [time - window, time + window]
func (p *KlineProcessor) GetContextWindow(eventTime time.Time, windowMinutes int) ([]model.KLineData, error) {
	// Calculate range
	start := eventTime.Add(-time.Duration(windowMinutes) * time.Minute)
	end := eventTime.Add(time.Duration(windowMinutes) * time.Minute)

	query := `
		SELECT time, close, volume
		FROM kline_1m
		WHERE time >= ? AND time <= ?
		ORDER BY time ASC
	`
	rows, err := p.duck.DB.Query(query, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var klines []model.KLineData
	for rows.Next() {
		var t time.Time
		var c, v float64
		if err := rows.Scan(&t, &c, &v); err != nil {
			return nil, err
		}
		// Convert back to KLineData
		// Note: Change is not recalculated here (expensive or needs LAG).
		// Usually for display chart we might need it, but for AI prompt, simple time/close/vol is enough.
		klines = append(klines, model.KLineData{
			Date:   t.Format("2006-01-02 15:04"),
			Close:  c,
			Amount: v,
		})
	}
	return klines, nil
}

// AnalysisEvent contains enriched anomaly data
type AnalysisEvent struct {
	Time        time.Time `json:"time"`
	Price       float64   `json:"price"`
	VolRatio    float64   `json:"vol_ratio"`
	RelativePos float64   `json:"relative_pos"` // 0-1 (Low to High of 30m)
	Bias30m     float64   `json:"bias_30m"`
	Note        string    `json:"note"` // AI friendly description
}

// AnalyzeVolatility performs On-the-fly Aggregation and Anomaly Detection
func (p *KlineProcessor) AnalyzeVolatility() ([]AnalysisEvent, error) {
	// SQL: Single Source of Truth (1m) -> Macro (30m) + Micro (1m) Join
	// DuckDB specific: epoch math for time_bucket (30m = 1800s)

	query := `
	WITH 
	-- 1. Macro Context: 30m Aggregation
	macro_context AS (
		SELECT 
			to_timestamp(floor(epoch(time)/1800)*1800) AS bucket_time,
			MAX(close)   AS k30_high,
			MIN(close)   AS k30_low,
			SUM(volume)  AS k30_vol,
			-- VWAP = Sum(P*V) / Sum(V)
			CASE WHEN SUM(volume) > 0 THEN SUM(close * volume) / SUM(volume) ELSE AVG(close) END AS k30_vwap
		FROM kline_1m
		GROUP BY 1
	),

	-- 2. Micro Signals: Anomaly Detection
	micro_signals AS (
		SELECT 
			time,
			close AS current_price,
			volume AS current_vol,
			to_timestamp(floor(epoch(time)/1800)*1800) AS link_bucket_time,
			-- Rolling Avg Vol (60 min window)
			AVG(volume) OVER (ORDER BY time ROWS BETWEEN 60 PRECEDING AND 1 PRECEDING) as roll_avg_vol
		FROM kline_1m
	)

	-- 3. Join & Feature Engineering
	SELECT 
		m.time,
		m.current_price,
		CASE WHEN m.roll_avg_vol > 0 THEN m.current_vol / m.roll_avg_vol ELSE 0 END as vol_ratio,
		
		-- Relative Position in 30m bar (0.0 - 1.0)
		(m.current_price - ctx.k30_low) / (ctx.k30_high - ctx.k30_low + 0.000001) as relative_pos,
		
		-- Bias from 30m VWAP (%)
		(m.current_price - ctx.k30_vwap) / (ctx.k30_vwap + 0.000001) * 100 as bias_30m

	FROM micro_signals m
	JOIN macro_context ctx ON m.link_bucket_time = ctx.bucket_time
	WHERE 
		m.current_vol > 3 * m.roll_avg_vol  -- Volume Spike
		OR 
		(m.current_vol > 1.5 * m.roll_avg_vol AND ABS(bias_30m) > 2) -- Moderate Vol but High Deviation
	ORDER BY m.time
	LIMIT 10; -- Focus on top events
	`

	rows, err := p.duck.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("analyze query failed: %w", err)
	}
	defer rows.Close()

	var events []AnalysisEvent
	for rows.Next() {
		var e AnalysisEvent
		if err := rows.Scan(&e.Time, &e.Price, &e.VolRatio, &e.RelativePos, &e.Bias30m); err != nil {
			return nil, err
		}

		// Generate Note
		desc := ""
		if e.VolRatio > 5 {
			desc += "天量(Vol>5x) "
		} else if e.VolRatio > 3 {
			desc += "巨量(Vol>3x) "
		}

		if e.RelativePos > 0.95 {
			desc += "冲击30m高点 "
		} else if e.RelativePos < 0.05 {
			desc += "跌穿30m低点 "
		} else if e.RelativePos > 0.4 && e.RelativePos < 0.6 {
			desc += "中枢震荡 "
		}

		if e.Bias30m > 3 {
			desc += "正乖离极大 "
		} else if e.Bias30m < -3 {
			desc += "负乖离极大 "
		}

		e.Note = desc
		events = append(events, e)
	}
	return events, nil
}
