package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DeepSeek   DeepSeekConfig `yaml:"deepseek"`
	HoldStocks []string       `yaml:"hold_stocks"`
	Output     OutputConfig   `yaml:"output"`

	StartTime  time.Time
	StartTsStr string

	// for analysis special
	HoldKlineReportFile string

	// for analysis all
	JsonFile              string
	DragonReportFile      string
	ReportTop3FileMD      string
	ReportTop1FileMD      string
	ReportWinnersFileMD   string
	ReportTop3FileHTML    string
	ReportTop1FileHTML    string
	ReportWinnersFileHTML string
}

type DeepSeekConfig struct {
	APIKey string `yaml:"api_key"`
}

type OutputConfig struct {
	Path string `yaml:"path"`
}

func InitOutputPath(outputPath string) error {
	// 1. 清理路径
	cleanPath := filepath.Clean(outputPath)

	// 2. 检查是否已经是有效目录
	if fi, err := os.Stat(cleanPath); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("%s 已存在但不是目录", cleanPath)
		}
		return nil // 目录已存在
	}

	// 3. 创建目录
	if err := os.MkdirAll(cleanPath, 0755); err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", cleanPath, err)
	}

	return nil
}

func LoadConfig() (*Config, error) {
	f, err := os.Open("config.yaml")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&cfg)
	if err != nil {
		return nil, err
	}

	// init deepseek api
	if cfg.DeepSeek.APIKey == "" {
		cfg.DeepSeek.APIKey = os.Getenv("DS_APIKEY_FOR_DRAGON")
	}

	// init output path
	if cfg.Output.Path == "" {
		cfg.Output.Path = filepath.Join(".", "output", time.Now().Format("2006-01-02"))

	} else {
		cfg.Output.Path = filepath.Join(cfg.Output.Path, time.Now().Format("2006-01-02"))
	}
	err = InitOutputPath(cfg.Output.Path)
	if err != nil {
		return &cfg, err
	}

	// init file name
	cfg.StartTime = time.Now()
	cfg.StartTsStr = cfg.StartTime.Format("2006-01-02T15-04-05")
	// for special
	cfg.HoldKlineReportFile = filepath.Join(cfg.Output.Path, fmt.Sprintf("Hold_Kline_Report_%s.html", cfg.StartTsStr))
	// for all
	cfg.JsonFile = filepath.Join(cfg.Output.Path, fmt.Sprintf("AI_Dragon_%s.json", cfg.StartTsStr))
	cfg.DragonReportFile = filepath.Join(cfg.Output.Path, fmt.Sprintf("DragonReport_%s.html", cfg.StartTsStr))
	cfg.ReportTop3FileMD = filepath.Join(cfg.Output.Path, fmt.Sprintf("DeepSeek_Fox_Top3_Report_%s.md", cfg.StartTsStr))
	cfg.ReportTop1FileMD = filepath.Join(cfg.Output.Path, fmt.Sprintf("DeepSeek_Fox_Top1_Report_%s.md", cfg.StartTsStr))
	cfg.ReportWinnersFileMD = filepath.Join(cfg.Output.Path, fmt.Sprintf("DeepSeek_Fox_Winners_Report_%s.md", cfg.StartTsStr))
	cfg.ReportTop3FileHTML = filepath.Join(cfg.Output.Path, fmt.Sprintf("DeepSeek_Fox_Top3_Report_%s.html", cfg.StartTsStr))
	cfg.ReportTop1FileHTML = filepath.Join(cfg.Output.Path, fmt.Sprintf("DeepSeek_Fox_Top1_Report_%s.html", cfg.StartTsStr))
	cfg.ReportWinnersFileHTML = filepath.Join(cfg.Output.Path, fmt.Sprintf("DeepSeek_Fox_Winners_Report_%s.html", cfg.StartTsStr))

	return &cfg, nil
}
