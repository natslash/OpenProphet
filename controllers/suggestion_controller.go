package controllers

import (
	"net/http"
	"prophet-trader/services"

	"github.com/gin-gonic/gin"
)

type SuggestionController struct {
	sm *services.SuggestionManager
}

func NewSuggestionController(sm *services.SuggestionManager) *SuggestionController {
	return &SuggestionController{sm: sm}
}

func (sc *SuggestionController) HandleListSuggestions(c *gin.Context) {
	status := c.Query("status")
	suggestions, err := sc.sm.ListAll(status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, suggestions)
}

func (sc *SuggestionController) HandleGetHistory(c *gin.Context) {
	suggestions, err := sc.sm.ListAll("")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var history []interface{}
	for _, s := range suggestions {
		if s.Status != "PENDING" {
			history = append(history, s)
		}
	}
	c.JSON(http.StatusOK, history)
}

func (sc *SuggestionController) HandleAcceptSuggestion(c *gin.Context) {
	id := c.Param("id")
	if err := sc.sm.AcceptSuggestion(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "accepted"})
}

func (sc *SuggestionController) HandleDismissSuggestion(c *gin.Context) {
	id := c.Param("id")
	if err := sc.sm.DismissSuggestion(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "dismissed"})
}

func (sc *SuggestionController) HandleGetTrackRecord(c *gin.Context) {
	record, err := sc.sm.GetTrackRecord()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}
