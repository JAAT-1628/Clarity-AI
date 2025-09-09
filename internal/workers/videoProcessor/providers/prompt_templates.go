package providers

import (
	"clarity-ai/internal/domain/models"
	"fmt"
)

type PromptTemplates struct{}

func NewPromptTemplates() *PromptTemplates {
	return &PromptTemplates{}
}

func (p *PromptTemplates) BuildSummaryPrompt(transcript string) string {
	return fmt.Sprintf(`Create a concise 2-3 sentence summary of this video segment:

  TRANSCRIPT:
  %s
  
  Focus on:
  - The main topic/concept discussed
  - Key takeaway or learning point
  
  Keep it brief and educational. Maximum 60 words.`, transcript)
}

func (p *PromptTemplates) BuildKeyTopicsPrompt(transcript, startTime, endTime string) string {
	return fmt.Sprintf(`Extract the ONE most important topic from this educational content:

TRANSCRIPT (%s - %s):
%s

Return complete topic information in JSON format:
{
  "topics": [
    {
      "title": "Clear, specific topic title",
      "description": "Educational description (1-2 sentences)",
      "key_points": ["Important point 1", "Key concept 2"],
      "why_important": "Why this topic matters for learning and understanding",
      "related_concepts": ["Related concept 1", "Connected topic 2"],
      "difficulty": "beginner"
    }
  ]
}

Requirements:
- Only 1 topic maximum
- Use difficulty levels: "beginner", "intermediate", "advanced"
- Include why_important and related_concepts
- Focus on educational value`, startTime, endTime, transcript)
}

func (p *PromptTemplates) BuildKeyQuestionsPrompt(transcript string, topics []*models.Topic, startTime, endTime string) string {
	return fmt.Sprintf(`Generate 1-2 high-quality questions about the most important concepts:
  
  TRANSCRIPT:
  %s
  
  Create questions that:
  - Test understanding of key concepts only
  - Have clear, correct answers
  - Are practically useful
  
  Respond in JSON format:
  {
    "questions": [
      {
        "question": "Clear, specific question?",
        "answer": "Complete, helpful answer",
        "difficulty": "easy|medium|hard",
        "type": "short_answer"
      }
    ]
  }
  
  Maximum 2 questions. Focus on quality and relevance.`, transcript)
}

func (p *PromptTemplates) BuildInsightsPrompt(transcript, startTime, endTime string) string {
	return fmt.Sprintf(`Extract key educational insights and valuable takeaways from this video transcript.

TRANSCRIPT (%s - %s):
%s

Identify insights that are:
- Educationally valuable and practical
- Non-obvious or particularly important for learning
- Clearly explained with educational context
- Relevant to the subject matter
- Actionable for students or learners

Look for these types of insights:
- Key concepts that are fundamental to understanding
- Practical tips or study methods mentioned
- Common misconceptions or mistakes to avoid
- Important principles, formulas, coding style, or techniques
- Learning strategies or educational advice

Respond in JSON format:
{
  "insights": [
    {
      "insight": "Clear, concise educational insight statement",
      "explanation": "Detailed explanation of why this is educationally important and how learners can apply or benefit from this knowledge",
      "type": "key_concept|practical_tip|common_mistake|study_method|learning_strategy"
    }
  ]
}

Provide 2-3 high-quality, educationally valuable insights.`, startTime, endTime, transcript)
}
