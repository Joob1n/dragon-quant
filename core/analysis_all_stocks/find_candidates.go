package core

import (
	"dragon-quant/config"
	"dragon-quant/data_processor"
	"dragon-quant/fetcher"
	"dragon-quant/model"
	"fmt"
	"sync"
)

type FindCandidatesResult struct {
	Candidates map[string]*model.StockInfo
}

func FindCandidates(cfg *config.Config, scanHotPointSectorsResult ScanHotPointSectorsResult) FindCandidatesResult {
	fmt.Println("🚀 [Step 2] 启动竞价资金初筛 (Price/Flow/CallAuction)...")

	candidates := make(map[string]*model.StockInfo)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, sec := range scanHotPointSectorsResult.AllSectors {
		wg.Add(1)
		go func(s model.SectorInfo) {
			defer wg.Done()
			// 🔥 f19:开盘金额(竞价), f62:净流入, f7:振幅
			stocks := fetcher.FetchSectorStocks(s.Code)

			for _, stk := range stocks {
				// Use the FilterBasic function
				if !data_processor.FilterBasic(stk) {
					continue
				}

				mu.Lock()
				if existing, exists := candidates[stk.Code]; exists {
					existing.Tags = append(existing.Tags, s.Name)
				} else {
					newStk := stk
					newStk.Tags = []string{s.Name}
					candidates[stk.Code] = &newStk
				}
				mu.Unlock()
			}
		}(sec)
	}
	wg.Wait()
	fmt.Printf("   -> 初筛入围: %d 只\n", len(candidates))

	return FindCandidatesResult{
		Candidates: candidates,
	}
}
