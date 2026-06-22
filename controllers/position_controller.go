package controllers

import (
	"net/http"
	"prophet-trader/config"
	"prophet-trader/services"

	"github.com/gin-gonic/gin"
)

// PositionManagementController handles managed position operations
type PositionManagementController struct {
	positionManager *services.PositionManager
}

// NewPositionManagementController creates a new position management controller
func NewPositionManagementController(positionManager *services.PositionManager) *PositionManagementController {
	return &PositionManagementController{
		positionManager: positionManager,
	}
}

// HandlePlaceManagedPosition handles placing a new managed position
// POST /api/v1/positions/managed
func (pmc *PositionManagementController) HandlePlaceManagedPosition(c *gin.Context) {
	if config.AppConfig.RequireDoubleConfirm {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Manual endpoints disabled when RequireDoubleConfirm is active. Use the Agent Intent flow.",
		})
		return
	}

	var req services.PlaceManagedPositionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return
	}

	position, err := pmc.positionManager.PlaceManagedPosition(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to place managed position",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Managed position created successfully",
		"position": position,
	})
}

// HandleGetManagedPosition retrieves a specific managed position
// GET /api/v1/positions/managed/:id
func (pmc *PositionManagementController) HandleGetManagedPosition(c *gin.Context) {
	positionID := c.Param("id")
	if positionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "position ID required",
		})
		return
	}

	position, err := pmc.positionManager.GetManagedPosition(positionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Position not found",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, position)
}

// HandleListManagedPositions lists all managed positions
// GET /api/v1/positions/managed?status=ACTIVE
func (pmc *PositionManagementController) HandleListManagedPositions(c *gin.Context) {
	status := c.Query("status")

	positions := pmc.positionManager.ListManagedPositions(status)

	c.JSON(http.StatusOK, gin.H{
		"count":     len(positions),
		"positions": positions,
	})
}

// HandleCloseManagedPosition manually closes a managed position
// DELETE /api/v1/positions/managed/:id
func (pmc *PositionManagementController) HandleCloseManagedPosition(c *gin.Context) {
	positionID := c.Param("id")
	if positionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "position ID required",
		})
		return
	}

	if err := pmc.positionManager.CloseManagedPosition(c.Request.Context(), positionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to close position",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Position closed successfully",
	})
}
