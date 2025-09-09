package models

import (
	"time"
)

type UserPlan string

const (
	PlanFree    UserPlan = "free"
	PlanPremium UserPlan = "premium"
	PlanPro     UserPlan = "pro"
)

type User struct {
	ID                int64     `json:"id" db:"id"`
	Username          string    `json:"username" db:"username"`
	Email             string    `json:"email" db:"email"`
	Password          string    `json:"-" db:"password"`
	Plan              UserPlan  `json:"plan" db:"plan"`
	APIUsageCount     int64     `json:"api_usage_count" db:"api_usage_count"`
	APIUsageResetDate time.Time `json:"api_usage_reset_date" db:"api_usage_reset_date"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

type AIProvider string

const (
	AIProviderFree   AIProvider = "AI_PROVIDER_FREE"
	AIProviderGemini AIProvider = "AI_PROVIDER_GEMINI"
	AIProviderOpenAI AIProvider = "AI_PROVIDER_OPENAI"
	AIProviderClaude AIProvider = "AI_PROVIDER_CLAUDE"
)
