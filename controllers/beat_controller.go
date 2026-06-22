package controllers

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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
	c.JSON(200, gin.H{"running": bc.beat.IsRunning()})
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
	bc.beat.InjectMessage(req.Message)
	c.JSON(200, gin.H{"status": "Message injected"})
}

func (bc *BeatController) HandleStreamLogs(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	file, err := os.Open("bot.log")
	if err != nil {
		c.SSEvent("error", fmt.Sprintf("failed to open log file: %v", err))
		return
	}
	defer file.Close()

	// Seek to end of file to tail
	file.Seek(0, io.SeekEnd)

	reader := bufio.NewReader(file)
	ctx := c.Request.Context()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					time.Sleep(500 * time.Millisecond)
					continue
				}
				return
			}
			line = strings.TrimSpace(line)
			if line != "" {
				c.SSEvent("log", line)
				c.Writer.Flush()
			}
		}
	}
}
