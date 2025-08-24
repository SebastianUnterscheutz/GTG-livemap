package main

import (
	"context"
	"flag"
	"fmt"
	"gtglivemap/api/handlers"
	"gtglivemap/api/middleware"
	"gtglivemap/cache"
	"gtglivemap/cmd/seeder"
	"gtglivemap/config"
	"gtglivemap/database"
	"gtglivemap/pkg/moderation"
	"gtglivemap/utils"
	"gtglivemap/worker"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func setup() {
	log.Println("--- Initializing shared components ---")
	config.LoadConfig()
	worker.RDB = redis.NewClient(&redis.Options{Addr: config.AppConfig.Redis.Addr, Password: config.AppConfig.Redis.Password, DB: config.AppConfig.Redis.DB})
	_, err := worker.RDB.Ping(context.Background()).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	log.Println("Redis connection established")
	cache.Init()
	database.Connect()
	// database.Migrate()
	database.Seed()
	handlers.InitAuth()
	utils.InitEncryption()
	log.Println("--- Shared components initialized ---")
}

func startWebServer(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("--- Starting Web Server ---")
	r := gin.Default()

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowCredentials = true
	corsConfig.AddAllowMethods("PUT", "PATCH", "DELETE")
	corsConfig.AddAllowHeaders("Authorization", "X-API-KEY", "Content-Type")
	r.Use(cors.New(corsConfig))

	// --- 1. API Routen ---
	apiLimiter, err := middleware.CreateRateLimiter("200-S")
	if err != nil {
		log.Fatalf("Could not create API rate limiter: %v", err)
	}
	authLimiter, err := middleware.CreateRateLimiter("20-M")
	if err != nil {
		log.Fatalf("Could not create Auth rate limiter: %v", err)
	}
	sensitiveLimiter, err := middleware.CreateRateLimiter("50-M") //
	if err != nil {
		log.Fatalf("Could not create sensitive action rate limiter: %v", err)
	}

	auth := r.Group("/auth")
	auth.Use(middleware.GinLimitMiddleware(authLimiter))
	{
		auth.GET("/discord/login", handlers.HandleDiscordLogin)
		auth.GET("/discord/callback", handlers.HandleDiscordCallback)
		auth.GET("/logout", handlers.HandleLogout)
	}

	apiV1 := r.Group("/api/v1")
	apiV1.Use(middleware.GinLimitMiddleware(apiLimiter))
	{

		apiV1.GET("/cdn/*path", handlers.CDNProxyHandler)
		apiV1.GET("/tiles/*path", handlers.TilesProxyHandler)

		authenticated := apiV1.Group("/")
		authenticated.Use(middleware.APIKeyAuthMiddleware())
		{
			authenticated.POST("/positions", handlers.PostPositionsHandler)
			authenticated.POST("/events/damage", handlers.PostDamageEventsHandler)
		}

		serverAPI := apiV1.Group("/server")
		serverAPI.Use(middleware.APIKeyAuthMiddleware())
		{
			// Routen für die Zugriffsverwaltung
			serverAPI.GET("/access", handlers.APIGetAccessListHandler)
			serverAPI.POST("/access", handlers.APIGrantAccessHandler)
			serverAPI.DELETE("/access/:user_id", handlers.APIRevokeAccessHandler)

			// Routen für die Kartenverwaltung
			serverAPI.GET("/map", handlers.APIGetCurrentMapHandler)
			serverAPI.PUT("/map", handlers.APISetCurrentMapHandler)
		}
		public := apiV1.Group("/public")
		{
			public.GET("/servers", handlers.GetPublicServersHandler)
			public.GET("/positions/timestamps/:server_id", handlers.GetTimestampsHandler)
			public.GET("/positions/:server_id/:timestamp", handlers.GetPositionsByTimeHandler)
			public.GET("/damage_events/:server_id/:timestamp", handlers.GetDamageEventsByTimeHandler)
			public.GET("/heatmap", handlers.GetHeatmapHandler)
			public.GET("/map-configs", handlers.GetMapConfigsHandler)
			public.POST("/players/names", handlers.GetPlayerNamesHandler)
			public.GET("/events/timestamps/:server_id", handlers.GetDamageEventTimestampsHandler)
			public.GET("/events/timestamps/:server_id/:guid", handlers.GetPlayerEventTimestampsHandler)
			public.GET("/avatars/:user_id/:avatar_hash", handlers.AvatarProxyHandler)
			public.GET("/positions/latest/:server_id", handlers.GetLatestPositionsHandler)
			public.GET("/damage_events/range/:server_id", handlers.GetDamageEventsInRangeHandler)
			public.GET("/demo-data", handlers.GetDemoDataHandler)
		}
		private := apiV1.Group("/")
		private.Use(middleware.SessionAuthMiddleware())
		{

			admin := private.Group("/")
			admin.Use(middleware.AdminOnlyMiddleware())
			{
				admin.PUT("/maps/:id", handlers.UpdateMapConfigHandler)
				admin.PUT("admin/users/:user_id/limit", handlers.AdminSetServerLimitHandler)
				admin.GET("admin/users", handlers.AdminGetAllUsersHandler)
				admin.GET("admin/users/:user_id/servers", handlers.AdminGetUserServersHandler)
				admin.GET("admin/settings/demo", handlers.AdminGetDemoSettingsHandler)
				admin.PUT("admin/settings/demo", handlers.AdminSetDemoSettingsHandler)
				admin.PATCH("admin/maps/:id/calibrate", handlers.AdminUpdateMapCalibrationHandler)
			}

			private.GET("/users/me", handlers.MeHandler)
			private.GET("/dashboard/servers", handlers.GetUserServersHandler)
			private.GET("/users/search", handlers.SearchUsersHandler)
			private.GET("/servers", handlers.GetUserServersHandler)
			private.POST("/servers", handlers.CreateServerHandler)
			private.GET("/dashboard/servers/statuses", handlers.GetDashboardServerStatusesHandler)

			serverSpecific := private.Group("/servers/:id")
			serverSpecific.Use(middleware.ServerOwnerOrAdminMiddleware())
			{
				serverSpecific.GET("/status", handlers.GetServerStatusHandler)

				serverSpecific.PUT("", middleware.GinLimitMiddleware(sensitiveLimiter), handlers.UpdateServerHandler)
				serverSpecific.DELETE("", middleware.GinLimitMiddleware(sensitiveLimiter), handlers.DeleteServerHandler)
				serverSpecific.PATCH("/new-key", middleware.GinLimitMiddleware(sensitiveLimiter), handlers.RegenerateAPIKeyHandler)
				serverSpecific.POST("/test-connection", middleware.GinLimitMiddleware(sensitiveLimiter), handlers.TestConnectionHandler)

				serverSpecific.GET("/access", handlers.GetAccessListHandler)
				serverSpecific.POST("/access", handlers.GrantAccessHandler)
				serverSpecific.DELETE("/access/:user_id", handlers.RevokeAccessHandler)
			}
		}
	}

	r.Static("/static", "./static")

	r.GET("/", func(c *gin.Context) { c.File("./static/index.html") })
	r.GET("/map", func(c *gin.Context) { c.File("./static/map.html") })
	r.GET("/map.html", func(c *gin.Context) { c.File("./static/map.html") })
	r.GET("/dashboard", func(c *gin.Context) { c.File("./static/dashboard.html") })
	r.GET("/dashboard.html", func(c *gin.Context) { c.File("./static/dashboard.html") })
	r.GET("/admin.html", func(c *gin.Context) { c.File("./static/admin.html") })
	r.GET("/admin", func(c *gin.Context) { c.File("./static/admin.html") })
	r.GET("/privacy.html", func(c *gin.Context) { c.File("./static/privacy.html") })
	r.GET("/privacy", func(c *gin.Context) { c.File("./static/privacy.html") })

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"code": "ENDPOINT_NOT_FOUND", "message": "API endpoint not found"})
			return
		}
		c.File("./static/index.html")
	})

	port := fmt.Sprintf(":%d", config.AppConfig.Server.Port)
	srv := &http.Server{Addr: port, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()
	log.Printf(">>>> Web server started on http://localhost%s/ <<<<", port)
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Println("Web server forced to shutdown:", err)
	}
	log.Println("Web server exited gracefully")
}

func startScheduler(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("--- Starting Scheduler (Producer) ---")
	s := worker.InitScheduler()
	s.StartAsync()
	<-ctx.Done()
	s.Stop()
	log.Println("Scheduler exited gracefully")
}

func startJobConsumer(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("--- Starting Job Consumer (Worker) ---")
	worker.StartJobConsumer(ctx)
	log.Println("Job Consumer exited gracefully")
}

func main() {
	pwd, _ := os.Getwd()
	log.Printf("Current Working Directory: %s", pwd)

	mode := flag.String("mode", "all", "Run mode: all, web, scheduler, consumer, seed-bad-words")
	flag.Parse()

	if *mode == "seed-bad-words" {
		log.Println("--- Initializing components for seeder ---")
		config.LoadConfig()
		database.Connect()
		seeder.SeedBadWords()
		log.Println("Seeding complete. Exiting.")
		return
	}

	setup()

	log.Printf("Starting application in '%s' mode.", *mode)

	ctx, stop := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received, stopping all services...")
		stop()
	}()

	if strings.Contains(*mode, "web") || *mode == "all" || strings.Contains(*mode, "consumer") {
		moderation.InitBadWordFilter()
	}

	if *mode == "all" || *mode == "scheduler" { // Nur starten, wenn der Webserver läuft
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.StartRecentTimestampsWorker(ctx)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.StartDamageEventTimestampsWorker(ctx)
		}()
	}

	switch *mode {
	case "all":
		wg.Add(5)
		go startWebServer(ctx, &wg)
		go startScheduler(ctx, &wg)
		go startJobConsumer(ctx, &wg)
		go startJobConsumer(ctx, &wg)
		go startJobConsumer(ctx, &wg)
	case "web":
		wg.Add(1)
		go startWebServer(ctx, &wg)
	case "scheduler":
		wg.Add(1)
		go startScheduler(ctx, &wg)
	case "consumer":
		wg.Add(1)
		go startJobConsumer(ctx, &wg)
	default:
		log.Fatalf("Unknown mode: '%s'. Use one of the available modes.", *mode)
	}

	wg.Wait()
	log.Println("Application shut down gracefully.")
}
