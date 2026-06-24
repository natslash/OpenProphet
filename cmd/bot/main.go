package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"prophet-trader/config"
	"prophet-trader/controllers"
	"prophet-trader/database"
	"prophet-trader/interfaces"
	"prophet-trader/services"
	"prophet-trader/tws"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func main() {
	// Load configuration
	if err := config.Load(); err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	cfg := config.AppConfig

	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	if cfg.EnableLogging {
		level, _ := logrus.ParseLevel(cfg.LogLevel)
		logger.SetLevel(level)
	}

	logger.Info("Starting Prophet Trader Bot...")

	// Initialize services
	logger.Info("Initializing services...")

	var tradingService interfaces.TradingService
	var dataService interfaces.DataService

	// Phase 6.1: Live execution boundary check
	if cfg.IBKRPort == 4001 && (!cfg.RequireDoubleConfirm || cfg.AdminToken == "" || !cfg.AllowLivePort) {
		logger.Fatalf("FATAL: Unattended live execution prohibited. Port 4001 requires ALLOW_LIVE_PORT=true, REQUIRE_DOUBLE_CONFIRM=true, and a valid ADMIN_TOKEN.")
	} else if cfg.IBKRPort != 4002 && cfg.IBKRPort != 4001 {
		logger.Fatalf("IBKR_PORT=%d refused — only 4002 (paper) or 4001 (live, guarded) are permitted.", cfg.IBKRPort)
	}
	logger.WithFields(logrus.Fields{
		"host": cfg.IBKRHost, "port": cfg.IBKRPort, "clientID": cfg.IBKRClientID,
	}).Info("Connecting to IB Gateway (paper)...")

	client := tws.NewClient(cfg.IBKRHost, cfg.IBKRPort, cfg.IBKRClientID)
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := client.Connect(connectCtx); err != nil {
		connectCancel()
		logger.Fatal("Failed to connect to IB Gateway (paper):", err)
	}
	connectCancel()
	logger.Info("Connected to IB Gateway (paper).")

	resolver := tws.NewContractResolver(client)

	// Wrap order placement in the kill-switch (default OFF until Phase 4.3e).
	ibkrTrading := services.NewIBKRTradingService(client, resolver)
	gated := services.NewGatedTradingService(ibkrTrading, cfg.TradingEnabled)
	tradingService = gated
	// Data service sets ReqMarketDataType(4) — live preferred, delayed-frozen fallback.
	ibkrData := services.NewIBKRDataService(client, resolver)
	dataService = ibkrData
	ibkrTrading.SetDataService(dataService)
	ibkrTrading.SubscribePositions()

	// Auto-reconnect loop: when IB Gateway drops (daily restart, network
	// issue), disable trading and retry with exponential backoff.
	go func() {
		backoff := 5 * time.Second
		const maxBackoff = 5 * time.Minute
		for {
			<-client.Closed()
			gated.Disable("IB Gateway connection closed")
			logger.Error("IB Gateway disconnected — attempting reconnect...")

			ibkrData.OnDisconnect()
			ibkrTrading.OnDisconnect()

			for {
				time.Sleep(backoff)
				logger.WithField("backoff", backoff).Info("Reconnecting to IB Gateway...")

				rctx, rcancel := context.WithTimeout(context.Background(), 15*time.Second)
				err := client.Reconnect(rctx)
				rcancel()

				if err != nil {
					logger.WithError(err).Error("Reconnect failed")
					backoff = min(backoff*2, maxBackoff)
					continue
				}

				logger.Info("Reconnected to IB Gateway successfully")
				ibkrData.OnReconnect()
				ibkrTrading.OnReconnect()
				gated.Enable("IB Gateway reconnected")
				backoff = 5 * time.Second
				break
			}
		}
	}()

	// Create storage service
	storageService, err := database.NewLocalStorage(cfg.DatabasePath)
	if err != nil {
		logger.Fatal("Failed to create storage service:", err)
	}

	// Create order controller
	orderController := controllers.NewOrderController(
		tradingService,
		dataService,
		storageService,
	)

	// Create news service and controller
	newsService := services.NewNewsService()
	newsController := controllers.NewNewsController(newsService)

	// Create economic feeds service and controller
	economicFeedsService := services.NewEconomicFeedsService()
	economicFeedsController := controllers.NewEconomicFeedsController(economicFeedsService)

	// Create Gemini service and intelligence controller
	geminiService := services.NewGeminiService(cfg.GeminiAPIKey)
	analysisService := services.NewTechnicalAnalysisService(dataService)
	stockAnalysisService := services.NewStockAnalysisService(dataService, newsService, geminiService)
	intelligenceController := controllers.NewIntelligenceController(newsService, geminiService, analysisService, stockAnalysisService, dataService)

	// Test account connection
	logger.Info("Testing broker connection...")
	if tradingService != nil {
		if account, err := orderController.GetAccount(); err != nil {
			logger.Warn("Failed to read account (trading may be unavailable):", err)
		} else {
			logger.WithFields(logrus.Fields{
				"cash":            account.Cash,
				"buying_power":    account.BuyingPower,
				"portfolio_value": account.PortfolioValue,
			}).Info("Successfully read broker account")
		}
	} else {
		logger.Warn("Trading service unavailable - API credentials may be invalid")
	}

	// Start background tasks
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create position manager
	positionManager := services.NewPositionManager(tradingService, dataService, storageService)
	positionController := controllers.NewPositionManagementController(positionManager)

	// Supervised autonomous beat (Phase 4.3e). Proposes trade intents via IntentManager.
	intentManager := services.NewIntentManager(cfg.IntentTTLSeconds, logger)
	go intentManager.StartSweeper(ctx)
	
	autonomousBeat := services.NewAutonomousBeat(dataService, positionManager, tradingService, logger, services.AutonomousBeatConfig{
		Interval:           time.Duration(cfg.BeatIntervalSecs) * time.Second,
		MaxDailyExecutions: cfg.BeatMaxDailyExecutions,
		LLMPollingEnabled:  cfg.LLMPollingEnabled,
		LLMPollingInterval: time.Duration(cfg.LLMPollingIntervalSecs) * time.Second,
	}, intentManager, cfg.RequireDoubleConfirm)
	autonomousBeat.SetResolver(resolver)

	intentManager.SetFeedbackCallback(func(intent *services.Intent, reason string) {
		msg := fmt.Sprintf("System feedback: Your trade intent (ID: %s, Symbol: %s, Side: %s) was %s.", intent.ID, intent.Symbol, intent.Side, reason)
		autonomousBeat.InjectMessage(msg)
	})
	
	beatController := controllers.NewBeatController(autonomousBeat)

	// Create activity logger
	activityLogDir := os.Getenv("ACTIVITY_LOG_DIR")
	if activityLogDir == "" {
		activityLogDir = "./activity_logs"
	}
	activityLogger := services.NewActivityLogger(activityLogDir)
	activityController := controllers.NewActivityController(activityLogger)

	intentController := controllers.NewIntentController(intentManager, positionManager, tradingService, dataService, activityLogger)

	// Start trading session automatically
	if account, err := orderController.GetAccount(); err == nil {
		activityLogger.StartSession(ctx, account.PortfolioValue)
		logger.Info("Activity logging session started")
	}

	// Setup HTTP server
	router := setupRouter(orderController, newsController, intelligenceController, positionController, activityController, economicFeedsController, beatController, intentController)

	// Manual reconnect endpoint — triggers the same cleanup/reconnect/enable
	// flow as the automatic loop, but on-demand from the dashboard.
	router.POST("/api/v1/broker/reconnect", func(c *gin.Context) {
		logger.Info("Manual reconnect requested via API")
		gated.Disable("manual reconnect requested")
		ibkrData.OnDisconnect()
		ibkrTrading.OnDisconnect()

		rctx, rcancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer rcancel()
		if err := client.Reconnect(rctx); err != nil {
			logger.WithError(err).Error("Manual reconnect failed")
			c.JSON(500, gin.H{"error": "reconnect failed: " + err.Error()})
			return
		}
		ibkrData.OnReconnect()
		ibkrTrading.OnReconnect()
		gated.Enable("manual reconnect succeeded")
		logger.Info("Manual reconnect succeeded")
		c.JSON(200, gin.H{"status": "reconnected"})
	})

	// Broker status endpoint — reports whether the TWS connection is alive.
	router.GET("/api/v1/broker/status", func(c *gin.Context) {
		connected := client.IsConnected()
		tradingEnabled := gated.Enabled()
		c.JSON(200, gin.H{
			"connected":       connected,
			"trading_enabled": tradingEnabled,
			"trading_mode":    string(cfg.TradingMode),
		})
	})

	// Start the supervised beat only when explicitly enabled.
	if cfg.BeatEnabled {
		logger.Warn("Autonomous beat ENABLED — Starting native AI agent in background")
		go autonomousBeat.Start()
	}

	// Start data cleanup routine
	go startDataCleanup(ctx, storageService, cfg.DataRetentionDays, logger)

	// Start position monitor
	go startPositionMonitor(ctx, orderController, storageService, logger)

	// Start managed position monitoring
	go positionManager.MonitorPositions(ctx)

	// Setup graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdown
		logger.Info("Shutting down gracefully...")
		cancel()
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()

	// Start HTTP server
	logger.WithField("port", cfg.ServerPort).Info("Starting HTTP server...")
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		logger.Fatal("Failed to start server:", err)
	}
}

func setupRouter(orderController *controllers.OrderController, newsController *controllers.NewsController, intelligenceController *controllers.IntelligenceController, positionController *controllers.PositionManagementController, activityController *controllers.ActivityController, economicFeedsController *controllers.EconomicFeedsController, beatController *controllers.BeatController, intentController *controllers.IntentController) *gin.Engine {
	router := gin.Default()

	// Enable CORS
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// Trading endpoints
	api := router.Group("/api/v1")
	{
		// Order endpoints
		api.POST("/orders/buy", orderController.HandleBuy)
		api.POST("/orders/sell", orderController.HandleSell)
		api.DELETE("/orders/:id", orderController.HandleCancelOrder)
		api.GET("/orders", orderController.HandleGetOrders)

		// Position and account endpoints
		api.GET("/positions", orderController.HandleGetPositions)
		api.GET("/account", orderController.HandleGetAccount)

		// Market data endpoints
		api.GET("/market/quote/:symbol", orderController.HandleGetQuote)
		api.GET("/market/bar/:symbol", orderController.HandleGetBar)
		api.GET("/market/bars/:symbol", orderController.HandleGetBars)

		// Options trading endpoints
		api.POST("/options/order", orderController.PlaceOptionsOrder)
		api.GET("/options/positions", orderController.ListOptionsPositions)
		api.GET("/options/position/:symbol", orderController.GetOptionsPosition)
		api.GET("/options/chain/:symbol", orderController.GetOptionsChain)

		// News endpoints
		api.GET("/news", newsController.HandleGetNews)
		api.GET("/news/topic/:topic", newsController.HandleGetNewsByTopic)
		api.GET("/news/search", newsController.HandleSearchNews)
		api.GET("/news/market", newsController.HandleGetMarketNews)

		// MarketWatch endpoints
		api.GET("/news/marketwatch/topstories", newsController.HandleGetMarketWatchTopStories)
		api.GET("/news/marketwatch/realtime", newsController.HandleGetMarketWatchRealtimeHeadlines)
		api.GET("/news/marketwatch/bulletins", newsController.HandleGetMarketWatchBulletins)
		api.GET("/news/marketwatch/marketpulse", newsController.HandleGetMarketWatchMarketPulse)
		api.GET("/news/marketwatch/all", newsController.HandleGetAllMarketWatchNews)

		// Intelligence endpoints (AI-powered)
		api.POST("/intelligence/cleaned-news", intelligenceController.HandleGetCleanedNews)
		api.GET("/intelligence/quick-market", intelligenceController.HandleGetQuickMarketIntelligence)
		api.GET("/intelligence/analyze/:symbol", intelligenceController.HandleAnalyzeStock)
		api.POST("/intelligence/analyze-multiple", intelligenceController.HandleAnalyzeMultipleStocks)

		// Native AI Agent endpoints
		api.POST("/agent/start", beatController.HandleStart)
		api.POST("/agent/stop", beatController.HandleStop)
		api.GET("/agent/status", beatController.HandleStatus)
		api.POST("/agent/message", beatController.HandleMessage)
		api.GET("/agent/stream", beatController.HandleStreamLogs)

		// Intent authorization endpoints (Phase 6.1)
		api.GET("/beat/intents", intentController.HandleGetIntents)
		api.POST("/beat/authorize/:id", intentController.HandleAuthorizeIntent)
		api.POST("/beat/reject/:id", intentController.HandleRejectIntent)

		// Position management endpoints
		api.POST("/positions/managed", positionController.HandlePlaceManagedPosition)
		api.GET("/positions/managed", positionController.HandleListManagedPositions)
		api.GET("/positions/managed/:id", positionController.HandleGetManagedPosition)
		api.DELETE("/positions/managed/:id", positionController.HandleCloseManagedPosition)

		// Activity logging endpoints
		// Economic intelligence feeds (free, no API key required)
		api.GET("/feeds/treasury", economicFeedsController.HandleGetTreasury)
		api.GET("/feeds/gdelt", economicFeedsController.HandleGetGDELT)
		api.GET("/feeds/bls", economicFeedsController.HandleGetBLS)
		api.GET("/feeds/yfinance", economicFeedsController.HandleGetYFinance)
		api.GET("/feeds/usaspending", economicFeedsController.HandleGetUSASpending)
		api.GET("/feeds/comtrade", economicFeedsController.HandleGetComtrade)

		api.GET("/activity/current", activityController.HandleGetCurrentActivity)
		api.GET("/activity/:date", activityController.HandleGetActivityByDate)
		api.GET("/activity", activityController.HandleListActivityLogs)
		api.POST("/activity/session/start", activityController.HandleStartSession)
		api.POST("/activity/session/end", activityController.HandleEndSession)
		api.POST("/activity/log", activityController.HandleLogActivity)
	}

	// Serve dashboard
	router.Static("/dashboard", "./web")

	return router
}

// Background task to clean up old data
func startDataCleanup(ctx context.Context, storage interfaces.StorageService, retentionDays int, logger *logrus.Logger) {
	ticker := time.NewTicker(24 * time.Hour) // Run daily
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().AddDate(0, 0, -retentionDays)
			logger.WithField("cutoff", cutoff).Info("Running data cleanup")

			if err := storage.CleanupOldData(cutoff); err != nil {
				logger.WithError(err).Error("Failed to cleanup old data")
			}
		}
	}
}

// Background task to monitor and save positions
func startPositionMonitor(ctx context.Context, orderController *controllers.OrderController, storage *database.LocalStorage, logger *logrus.Logger) {
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get current positions
			positions, err := orderController.GetPositions()
			if err != nil {
				logger.WithError(err).Error("Failed to get positions")
				continue
			}

			// Save position snapshots
			for _, position := range positions {
				if err := storage.SavePosition(position); err != nil {
					logger.WithError(err).Error("Failed to save position snapshot")
				}
			}

			// Get and save account snapshot
			if account, err := orderController.GetAccount(); err == nil {
				if err := storage.SaveAccountSnapshot(account); err != nil {
					logger.WithError(err).Error("Failed to save account snapshot")
				}
			}

			logger.WithField("positions", len(positions)).Debug("Position monitor update complete")
		}
	}
}
