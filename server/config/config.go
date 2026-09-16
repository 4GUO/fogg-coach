package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config 全部配置走 .env（system-design §3.5：数值禁止硬编码）
type Config struct {
	Port        string
	Debug       bool
	JWTSecret   string
	JWTTTLDays  int
	LLMBaseURL  string
	LLMAPIKey   string
	LLMModel    string
	WXAppID     string
	WXSecret    string
	WXMock      bool
	Quota       Quota
	Abuse       Abuse
	GlobalCostCap float64 // 元/日，0=off
}

type Quota struct {
	DailyMessages  int // 50
	SessionTurns   int // 25
	MessageMaxLen  int // 500
	PlanGenerations int // 3
}

type Abuse struct {
	MsgsPerSec   int // 2
	CooldownSec  int // 30
	LongMsgStreak int // 3
}

var cfg *Config

// Load 加载 .env（不存在则跳过，依赖真实环境变量）
func Load() *Config {
	_ = godotenv.Load()
	i := func(k string, d int) int {
		if v := os.Getenv(k); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
		return d
	}
	f := func(k string, d float64) float64 {
		if v := os.Getenv(k); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				return n
			}
		}
		return d
	}
	cfg = &Config{
		Port:           envOr("PORT", "8080"),
		Debug:          os.Getenv("DEBUG") == "1",
		JWTSecret:      envOr("JWT_SECRET", "dev-secret"),
		JWTTTLDays:     i("JWT_TTL_DAYS", 7),
		LLMBaseURL:     os.Getenv("LLM_BASE_URL"),
		LLMAPIKey:      os.Getenv("LLM_API_KEY"),
		LLMModel:       os.Getenv("LLM_MODEL"),
		WXAppID:        os.Getenv("WX_APPID"),
		WXSecret:       os.Getenv("WX_SECRET"),
		WXMock:         os.Getenv("WX_MOCK") == "1",
		Quota:          Quota{DailyMessages: i("QUOTA_DAILY_MESSAGES", 50), SessionTurns: i("QUOTA_SESSION_TURNS", 25), MessageMaxLen: i("QUOTA_MESSAGE_MAX_LEN", 500), PlanGenerations: i("QUOTA_PLAN_GENERATIONS", 3)},
		Abuse:          Abuse{MsgsPerSec: i("RATE_LIMIT_MSGS_PER_SEC", 2), CooldownSec: i("RATE_LIMIT_COOLDOWN_SEC", 30), LongMsgStreak: i("LONG_MSG_STREAK", 3)},
		GlobalCostCap:  f("GLOBAL_DAILY_COST_CAP", 0),
	}
	if cfg.JWTSecret == "dev-secret" {
		log.Println("[warn] JWT_SECRET 未设置，使用开发默认值（生产必须配置）")
	}
	return cfg
}

func Get() *Config { return cfg }

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
