package controllers

import (
	"fmt"

	"prophet-trader/services"

	"github.com/gin-gonic/gin"
)

type BeatController struct {
	beat *services.AutonomousBeat
}

func NewBeatController(beat *services.AutonomousBeat) *BeatController {
	return &BeatController{beat: beat}
}

func (bc *BeatController) HandleStart(c *gin.Context) {
	if bc.beat.IsRunning() {
		c.JSON(400, gin.H{"error": "Agent is already running"})
		return
	}
	err := bc.beat.Start()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "Agent started"})
}

func (bc *BeatController) HandleStop(c *gin.Context) {
	if !bc.beat.IsRunning() {
		c.JSON(400, gin.H{"error": "Agent is not running"})
		return
	}
	err := bc.beat.Stop()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "Agent stopped"})
}

func (bc *BeatController) HandleStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"running":          bc.beat.IsRunning(),
		"paused":           bc.beat.IsPaused(),
		"heartbeatSeconds": int(bc.beat.Interval().Seconds()),
		"beatCount":        bc.beat.BeatCount(),
		"stats":            bc.beat.Stats(),
	})
}

func (bc *BeatController) HandleMessage(c *gin.Context) {
	var req struct {
		Message string `json:"message"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}
	if req.Message == "" {
		c.JSON(400, gin.H{"error": "Message is required"})
		return
	}
	if !bc.beat.IsRunning() {
		c.JSON(400, gin.H{"error": "Agent is not running. Please start the agent first."})
		return
	}
	bc.beat.InjectDirectMessage(req.Message)
	c.JSON(200, gin.H{"status": "Message injected"})
}

func (bc *BeatController) HandleStreamLogs(c *gin.Context) {
	if services.Hub == nil {
		c.JSON(500, gin.H{"error": "SSE hub not initialized"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()

	ch := services.Hub.Subscribe()
	defer services.Hub.Unsubscribe(ch)

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Event, string(evt.Data)))
			c.Writer.Flush()
		}
	}
}
