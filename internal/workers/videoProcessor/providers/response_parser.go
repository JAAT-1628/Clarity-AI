package providers

import (
	"clarity-ai/internal/domain/models"
	"encoding/json"
	"log"
	"strings"
)

type ResponseParser struct{}

func NewResponseParser() *ResponseParser {
	return &ResponseParser{}
}

func (p *ResponseParser) ParseSummary(response string) string {
	summary := strings.TrimSpace(response)
	if strings.HasPrefix(summary, "{") {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(summary), &parsed); err == nil {
			if s, ok := parsed["summary"].(string); ok {
				return s
			}
		}
	}
	return summary
}

func (p *ResponseParser) ParseTopics(response, startTime, endTime string) ([]*models.Topic, error) {
	var parsed struct {
		Topics []struct {
			Title           string   `json:"title"`
			Description     string   `json:"description"`
			KeyPoints       []string `json:"key_points"`
			WhyImportant    string   `json:"why_important"`
			RelatedConcepts []string `json:"related_concepts"`
			Difficulty      string   `json:"difficulty"`
		} `json:"topics"`
	}

	jsonStr := p.extractJSON(response)
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		log.Printf("Failed to parse topics JSON: %v", err)
		return p.fallbackParseTopics(response, startTime, endTime), nil
	}

	topics := make([]*models.Topic, len(parsed.Topics))
	for i, t := range parsed.Topics {
		whyImportant := t.WhyImportant
		if whyImportant == "" {
			whyImportant = "Important for understanding the main concepts discussed in this segment"
		}

		relatedConcepts := t.RelatedConcepts
		if len(relatedConcepts) == 0 {
			relatedConcepts = []string{"Study this topic further", "Practice related problems"}
		}

		difficulty := t.Difficulty
		if difficulty == "" {
			difficulty = "intermediate"
		}

		topics[i] = &models.Topic{
			Title:           t.Title,
			Description:     t.Description,
			KeyPoints:       t.KeyPoints,
			TimestampStart:  startTime,
			TimestampEnd:    endTime,
			WhyImportant:    whyImportant,
			RelatedConcepts: relatedConcepts,
			Difficulty:      difficulty,
		}
	}

	return topics, nil
}

func (p *ResponseParser) ParseQuestions(response string, topics []*models.Topic, startTime string) ([]*models.Question, error) {
	var parsed struct {
		Questions []struct {
			Question   string `json:"question"`
			Answer     string `json:"answer"`
			Difficulty string `json:"difficulty"`
			Type       string `json:"type"`
		} `json:"questions"`
	}

	jsonStr := p.extractJSON(response)
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		log.Printf("Failed to parse questions JSON: %v", err)
		return p.fallbackParseQuestions(response, topics, startTime), nil
	}

	questions := make([]*models.Question, len(parsed.Questions))
	for i, q := range parsed.Questions {
		difficulty := q.Difficulty
		if difficulty == "" {
			difficulty = "medium"
		}

		qType := q.Type
		if qType == "" {
			qType = "short_answer"
		}

		questions[i] = &models.Question{
			Question:           q.Question,
			Answer:             q.Answer,
			Difficulty:         difficulty,
			Type:               qType,
			TimestampReference: startTime,
		}

		if len(topics) > 0 {
			questions[i].TopicID = &topics[0].ID
		}
	}

	return questions, nil
}

func (p *ResponseParser) ParseInsights(response, startTime string) ([]*models.KeyInsight, error) {
	var parsed struct {
		Insights []struct {
			Insight     string `json:"insight"`
			Explanation string `json:"explanation"`
			Type        string `json:"type"`
		} `json:"insights"`
	}

	jsonStr := p.extractJSON(response)
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return p.fallbackParseInsights(response, startTime), nil
	}

	insights := make([]*models.KeyInsight, len(parsed.Insights))
	for i, ins := range parsed.Insights {
		insights[i] = &models.KeyInsight{
			Insight:     ins.Insight,
			Explanation: ins.Explanation,
			Timestamp:   startTime,
			Type:        ins.Type,
		}
	}

	return insights, nil
}

// Helper methods
func (p *ResponseParser) extractJSON(response string) string {
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start != -1 && end != -1 && end > start {
		return response[start : end+1]
	}
	return response
}

func (p *ResponseParser) fallbackParseTopics(response, startTime, endTime string) []*models.Topic {
	return []*models.Topic{
		{
			Title:           "Main Topic",
			Description:     "Key concept discussed in this segment",
			KeyPoints:       []string{"Important information covered"},
			TimestampStart:  startTime,
			TimestampEnd:    endTime,
			WhyImportant:    "Essential for understanding the subject matter",
			RelatedConcepts: []string{"Further study recommended"},
			Difficulty:      "intermediate",
		},
	}
}

func (p *ResponseParser) fallbackParseQuestions(response string, topics []*models.Topic, startTime string) []*models.Question {
	questions := []*models.Question{}
	for _, topic := range topics {
		question := &models.Question{
			Question:           "What are the key points about " + topic.Title + "?",
			Answer:             "The key points include: " + strings.Join(topic.KeyPoints, ", "),
			Difficulty:         "medium",
			Type:               "short_answer",
			TimestampReference: startTime,
			TopicID:            &topic.ID,
		}
		questions = append(questions, question)
	}
	return questions
}

func (p *ResponseParser) fallbackParseInsights(response, startTime string) []*models.KeyInsight {
	return []*models.KeyInsight{
		{
			Insight:     "Important concept discussed in this segment",
			Explanation: "This segment contains valuable information worth reviewing",
			Timestamp:   startTime,
			Type:        "key_concept",
		},
	}
}
