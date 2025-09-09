package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	AI       AIConfig
	JWT      JWTConfig
	Billing  BillingConfig
}

type ServerConfig struct {
	Port          string
	Environment   string
	GRPCPort      string
	VideoGRPCPort string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type AIConfig struct {
	OpenAIKey    string
	ClaudeKey    string
	GeminiKey    string
	GroqKey      string
	WhisperModel string
	LlamaModel   string
}

type JWTConfig struct {
	Secret     string
	Expiration int
}

type BillingConfig struct {
	StripeSecret    string
	BillingGRPCPort string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	jwtExp, _ := strconv.Atoi(getEnv("JWT_EXPIRATION", "3600"))

	return &Config{
		Server: ServerConfig{
			Port:          getEnv("PORT", "8080"),
			Environment:   getEnv("ENVIRONMENT", "development"),
			GRPCPort:      getEnv("GRPC_PORT", "50051"),
			VideoGRPCPort: getEnv("VIDEO_GRPC_PORT", "50052"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "video_analyzer"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       redisDB,
		},
		AI: AIConfig{
			OpenAIKey:    getEnv("OPENAI_API_KEY", ""),
			ClaudeKey:    getEnv("CLAUDE_API_KEY", ""),
			GeminiKey:    getEnv("GOOGLE_API_KEY", ""),
			GroqKey:      getEnv("GROQ_API_KEY", ""),
			WhisperModel: getEnv("WHISPER_MODEL", "base"),
			LlamaModel:   getEnv("LLAMA_MODEL", "llama3"),
		},
		JWT: JWTConfig{
			Secret:     getEnv("JWT_SECRET", "th*s*sASEcretKeyR*PT*$"),
			Expiration: jwtExp,
		},
		Billing: BillingConfig{
			StripeSecret:    getEnv("STRIPE_SECRET", ""),
			BillingGRPCPort: getEnv("BILLING_GRPC_PORT", "50053"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
