package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// GeminiService handles interactions with Google's Gemini AI API
type GeminiService struct {
	apiKey     string
	httpClient *http.Client
	model      string
}

// GeminiRequest represents a request to Gemini API
type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

// GeminiContent represents content in the request
type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart represents a part of the content
type GeminiPart struct {
	Text string `json:"text"`
}

// GeminiResponse represents the response from Gemini API
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// CleanedNews represents a token-efficient news summary
type CleanedNews struct {
	GeneratedAt      time.Time         `json:"generated_at"`
	SourceCount      int               `json:"source_count"`
	ArticleCount     int               `json:"article_count"`
	MarketSentiment  string            `json:"market_sentiment"`
	KeyThemes        []string          `json:"key_themes"`
	StockMentions    map[string]string `json:"stock_mentions"`
	ActionableItems  []string          `json:"actionable_items"`
	ExecutiveSummary string            `json:"executive_summary"`
	FullAnalysis     string            `json:"full_analysis"`
}

// NewGeminiService creates a new Gemini service
func NewGeminiService(apiKey string) *GeminiService {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}

	// gemini-2.0-flash was retired; default to a current flash model, overridable
	// via NEWS_CLEAN_MODEL.
	model := os.Getenv("NEWS_CLEAN_MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &GeminiService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		model: model,
	}
}

// CleanNewsForTrading takes raw news items and creates a token-efficient summary
// optimized for trading decisions
func (gs *GeminiService) CleanNewsForTrading(newsItems []NewsItem) (*CleanedNews, error) {
	if len(newsItems) == 0 {
		return nil, fmt.Errorf("no news items provided")
	}

	// Build the news text
	var newsText strings.Builder
	for i, item := range newsItems {
		newsText.WriteString(fmt.Sprintf("[%d] %s\n", i+1, item.Title))
		if item.Description != "" {
			// Clean HTML tags from description
			cleanDesc := strings.ReplaceAll(item.Description, "<", "")
			cleanDesc = strings.ReplaceAll(cleanDesc, ">", "")
			newsText.WriteString(fmt.Sprintf("   %s\n", cleanDesc[:min(200, len(cleanDesc))]))
		}
		newsText.WriteString(fmt.Sprintf("   Source: %s | Published: %s\n\n", item.Source, item.PubDate))
	}

	// Create a trading-focused prompt
	prompt := fmt.Sprintf(`You are a financial analyst AI. Analyze the following %d news articles and create a CONCISE trading intelligence report.

NEWS ARTICLES:
%s

Provide a JSON response with this EXACT structure:
{
  "market_sentiment": "BULLISH|BEARISH|NEUTRAL",
  "key_themes": ["theme1", "theme2", "theme3"],
  "stock_mentions": {
    "SYMBOL": "POSITIVE|NEGATIVE|NEUTRAL with 1-sentence reason"
  },
  "actionable_items": ["brief actionable insight 1", "brief actionable insight 2"],
  "executive_summary": "2-3 sentence summary of the market situation"
}

Focus on:
- Stock symbols and their sentiment
- Market-moving themes
- Actionable trading insights
- Overall market direction

Keep it BRIEF and DENSE. Maximum 200 tokens total.`, len(newsItems), newsText.String())

	// Call Gemini
	response, err := gs.generateContent(prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	// Parse the JSON response
	var cleanedNews CleanedNews
	cleanedNews.GeneratedAt = time.Now()
	cleanedNews.SourceCount = countUniqueSources(newsItems)
	cleanedNews.ArticleCount = len(newsItems)
	cleanedNews.FullAnalysis = response

	// Try to extract JSON from the response
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		jsonStr := response[jsonStart : jsonEnd+1]
		var parsed struct {
			MarketSentiment  string            `json:"market_sentiment"`
			KeyThemes        []string          `json:"key_themes"`
			StockMentions    map[string]string `json:"stock_mentions"`
			ActionableItems  []string          `json:"actionable_items"`
			ExecutiveSummary string            `json:"executive_summary"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
			cleanedNews.MarketSentiment = parsed.MarketSentiment
			cleanedNews.KeyThemes = parsed.KeyThemes
			cleanedNews.StockMentions = parsed.StockMentions
			cleanedNews.ActionableItems = parsed.ActionableItems
			cleanedNews.ExecutiveSummary = parsed.ExecutiveSummary
		}
	}

	return &cleanedNews, nil
}

// generateContent calls the Gemini API
func (gs *GeminiService) generateContent(prompt string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		gs.model, gs.apiKey)

	reqBody := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{
					{Text: prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := gs.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return geminiResp.Candidates[0].Content.Parts[0].Text, nil
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func countUniqueSources(items []NewsItem) int {
	sources := make(map[string]bool)
	for _, item := range items {
		if item.Source != "" {
			sources[item.Source] = true
		}
	}
	return len(sources)
}
