package data_processor

import (
	"dragon-quant/model"
	"testing"
	"time"
)

func TestKlineProcessor(t *testing.T) {
	// 1. Setup DuckDB
	duck, err := NewDuckDB("")
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer duck.Close()

	proc := NewKlineProcessor(duck)

	// 2. Generate Synthetic Data
	// 100 points. Normal: Vol=100. Spike at 50: Vol=5000.
	var klines []model.KLineData
	startTime, _ := time.Parse("2006-01-02 15:04", "2026-01-01 10:00") // Starting at 10:00

	for i := 0; i < 100; i++ {
		curTime := startTime.Add(time.Duration(i) * time.Minute)
		vol := 100.0
		closePrice := 10.0

		// Inject Spike at index 50
		if i == 50 {
			vol = 5000.0
			closePrice = 11.0 // 10% jump
		}

		klines = append(klines, model.KLineData{
			Date:   curTime.Format("2006-01-02 15:04"),
			Close:  closePrice,
			Amount: vol,
		})
	}

	// 3. Load Data
	err = proc.LoadData(klines)
	if err != nil {
		t.Fatalf("LoadData failed: %v", err)
	}

	// 4. Detect Anomalies
	events, err := proc.DetectAnomalies()
	if err != nil {
		t.Fatalf("DetectAnomalies failed: %v", err)
	}

	t.Logf("Detected %d anomalies", len(events))
	for _, e := range events {
		t.Logf(" - Time: %s, Vol: %.0f, Reason: %s", e.Time.Format("15:04"), e.Volume, e.Reason)
	}

	if len(events) == 0 {
		t.Fatal("Expected at least 1 anomaly (the spike), got 0")
	}

	// Verify top 1 is index 50 (10:50)
	top := events[0]
	// 10:00 + 50min = 10:50
	expectedTime := startTime.Add(50 * time.Minute)
	if !top.Time.Equal(expectedTime) {
		t.Errorf("Expected top anomaly at %v, got %v", expectedTime, top.Time)
	}

	// 5. Get Context Window
	// Get window around 10:50, +/- 5 mins
	windowData, err := proc.GetContextWindow(top.Time, 5)
	if err != nil {
		t.Fatalf("GetContextWindow failed: %v", err)
	}

	t.Logf("Context Window (Radius 5m): %d bars", len(windowData))
	// Should be 11 bars (center + 5 left + 5 right) if boundaries allow
	if len(windowData) != 11 {
		t.Errorf("Expected 11 bars in window, got %d", len(windowData))
	}

	// Check if spike is in window
	foundSpike := false
	for _, k := range windowData {
		if k.Amount > 4000 {
			foundSpike = true
		}
	}
	if !foundSpike {
		t.Error("Spike not found in context window data")
	}
}
