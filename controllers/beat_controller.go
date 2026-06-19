package controllers

import (
	"crypto/subtle"
	"strings"

	"prophet-trader/services"

	"github.com/gin-gonic/gin"
)

// BeatController exposes the supervised-beat endpoints. The authorize endpoint
// is the human-in-the-loop gate: it requires a bearer admin token the Node
// agent does not have, and fails closed if no token is configured.
type BeatController struct {
	beat       *services.AutonomousBeat
	adminToken string
}

func NewBeatController(beat *services.AutonomousBeat, adminToken string) *BeatController {
	return &BeatController{beat: beat, adminToken: adminToken}
}

// HandleListIntents returns pending intents (read-only; for the human to find
// the intent id to authorize).
func (bc *BeatController) HandleListIntents(c *gin.Context) {
	c.JSON(200, gin.H{"intents": bc.beat.ListIntents()})
}

// HandleAuthorize executes a pending intent after verifying the admin token.
func (bc *BeatController) HandleAuthorize(c *gin.Context) {
	// Fail closed: with no admin token configured, no intent can be authorized.
	if bc.adminToken == "" {
		c.JSON(403, gin.H{"error": "authorization disabled: ADMIN_TOKEN is not configured"})
		return
	}
	const prefix = "Bearer "
	auth := c.GetHeader("Authorization")
	if !strings.HasPrefix(auth, prefix) ||
		subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, prefix)), []byte(bc.adminToken)) != 1 {
		c.JSON(401, gin.H{"error": "unauthorized: valid admin bearer token required"})
		return
	}

	id := c.Param("intent_id")
	pos, err := bc.beat.Authorize(c.Request.Context(), id)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "executed", "position": pos})
}
