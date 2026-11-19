package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oneRole/config"
	"github.com/oneRole/connectors/job_sources"
	"github.com/oneRole/controller"
	dbconn "github.com/oneRole/db"
	"github.com/oneRole/middleware"
	"github.com/oneRole/migrations"
	"github.com/oneRole/repository"
	"github.com/oneRole/service"
	"github.com/oneRole/worker"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("OneRole stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := loadConfig(args)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startupCtx, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()

	client, database, err := dbconn.Open(startupCtx, dbconn.Config{
		URI: cfg.DatabaseURL, MaxPoolSize: 16, MinPoolSize: 4,
		ConnMaxIdleTime: 5 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("initialize MongoDB: %w", err)
	}
	defer client.Disconnect(context.Background())

	if err := migrations.Apply(startupCtx, database); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	slog.Info("OneRole database migrations and collection indexes are current")

	migrationCtx, cancelMigration := context.WithTimeout(ctx, 10*time.Minute)
	defer cancelMigration()

	if cfg.Mode == config.ModeMigrate || os.Getenv("MIGRATE_COMPANIES") == "true" {
		slog.Info("Running full company migration from company.json...")
		compCount, srcCount, err := worker.MigrateCompaniesFromJSON(migrationCtx, database, "company.json")
		if err != nil {
			return fmt.Errorf("migrate company.json: %w", err)
		}
		slog.Info("Company migration completed successfully", "companies", compCount, "sources", srcCount)
		if cfg.Mode == config.ModeMigrate {
			return nil
		}
	} else {
		existingCount, _ := database.Collection("companies").CountDocuments(migrationCtx, bson.M{})
		if existingCount < 15000 {
			if _, err := os.Stat("company.json"); err == nil {
				slog.Info("Populating companies catalog from company.json...", "current_companies", existingCount)
				compCount, srcCount, err := worker.MigrateCompaniesFromJSON(migrationCtx, database, "company.json")
				if err != nil {
					slog.Error("Failed to migrate company.json", "error", err)
				} else {
					slog.Info("Company catalog populated successfully", "companies_added", compCount, "sources_added", srcCount)
				}
			}
		}
	}

	if cfg.Mode == config.ModeDiscover || os.Getenv("SYNC_DISCOVERY_ON_STARTUP") == "true" {
		slog.Info("Starting OneRole discovery sync for all configured companies...")
		discoveryResult, err := worker.RunDiscoveryForAll(ctx, database)
		if err != nil {
			slog.Error("Discovery sync encountered an issue", "error", err)
		} else {
			slog.Info("Discovery sync completed successfully",
				"total_companies", discoveryResult.TotalCompanies,
				"total_sources", discoveryResult.TotalSources,
				"synced_sources", discoveryResult.SyncedSources,
				"total_jobs", discoveryResult.TotalJobs,
			)
			for company, count := range discoveryResult.JobsPerCompany {
				slog.Info("Company jobs cataloged in MongoDB", "company", company, "jobs_count", count)
			}
		}
		if cfg.Mode == config.ModeDiscover {
			return nil
		}
	} else {
		// Run discovery in background so HTTP API starts immediately
		go func() {
			slog.Info("OneRole background discovery sync active")
			_, _ = worker.RunDiscoveryForAll(ctx, database)
		}()
	}

	applicationStore := repository.NewApplicationWorkflowStore(database)
	taskService, err := service.NewDurableApplicationTaskService(applicationStore)
	if err != nil {
		return fmt.Errorf("initialize application task service: %w", err)
	}

	switch cfg.Mode {
	case config.ModeAPI:
		natsConn, _, _ := connectJetStream(cfg)
		if natsConn != nil {
			defer natsConn.Close()
		}
		handler, err := buildAPIHandler(startupCtx, cfg, database, natsConn, applicationStore)
		if err != nil {
			return err
		}
		return serveHTTP(ctx, cfg, handler)
	case config.ModeWorker:
		return runWorker(ctx, cfg, database, taskService)
	case config.ModeAll:
		natsConn, jetstreamClient, err := connectJetStream(cfg)
		if err != nil {
			slog.Warn("NATS not available; starting HTTP API server with MongoDB discovery catalog", "warning", err)
			handler, err := buildAPIHandler(startupCtx, cfg, database, nil, applicationStore)
			if err != nil {
				return err
			}
			return serveHTTP(ctx, cfg, handler)
		}
		defer natsConn.Close()
		workerRuntime, err := buildWorkerRuntime(cfg, database, taskService, jetstreamClient)
		if err != nil {
			return err
		}
		handler, err := buildAPIHandler(startupCtx, cfg, database, natsConn, applicationStore)
		if err != nil {
			return err
		}
		return runServices(ctx,
			namedService{name: "api", run: func(ctx context.Context) error { return serveHTTP(ctx, cfg, handler) }},
			namedService{name: "worker", run: workerRuntime.Run},
		)
	default:
		return fmt.Errorf("unsupported runtime mode %q", cfg.Mode)
	}
}

func loadConfig(args []string) (config.Config, error) {
	loadDotEnv()
	if len(args) == 0 {
		return config.Load()
	}
	if len(args) != 1 {
		return config.Config{}, errors.New("usage: onerole [api|worker|all|migrate|discover]")
	}
	mode := config.Mode(args[0])
	if mode != config.ModeAPI && mode != config.ModeWorker && mode != config.ModeAll && mode != config.ModeMigrate && mode != config.ModeDiscover {
		return config.Config{}, errors.New("usage: onerole [api|worker|all|migrate|discover]")
	}
	return config.LoadForMode(mode, os.LookupEnv)
}

func loadDotEnv() {
	if os.Getenv("DATABASE_URL") == "" {
		_ = os.Setenv("DATABASE_URL", "mongodb://localhost:27017/onerole")
	}
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}

func buildAPIHandler(ctx context.Context, cfg config.Config, database *mongo.Database, natsConn *nats.Conn,
	applicationStore *repository.ApplicationRepository) (http.Handler, error) {
	authRepositories := repository.NewAuthRepositories(database)
	providers, err := buildOAuthProviders(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize OAuth providers: %w", err)
	}
	auth, err := service.NewAuthService(service.AuthServiceConfig{
		PKCEEncryptionKey: cfg.AuthEncryptionKey, CSRFSigningKey: cfg.CSRFSigningKey,
		JWTIssuer: cfg.JWTIssuer, JWTAudience: cfg.JWTAudience, JWTSigningKeyID: cfg.JWTSigningKeyID,
		JWTSigningPrivateKey: cfg.JWTSigningPrivateKey, AccessTokenTTL: cfg.AccessTokenTTL,
		OAuthTransactionTTL: cfg.OAuthTransactionTTL, RefreshIdleTTL: cfg.RefreshIdleTTL,
		RefreshAbsoluteTTL: cfg.RefreshAbsoluteTTL, FrontendRedirectURL: cfg.FrontendAuthCallback,
		AllowedOrigins: cfg.AllowedOrigins,
	}, authRepositories.OAuthTransactions, authRepositories.Identities, authRepositories.Users,
		authRepositories.Sessions, authRepositories.Memberships, providers)
	if err != nil {
		return nil, fmt.Errorf("initialize authentication service: %w", err)
	}

	applications, err := service.NewApplicationService(applicationStore, cfg.IdempotencyHMACKey)
	if err != nil {
		return nil, fmt.Errorf("initialize application service: %w", err)
	}
	discovery := repository.NewDiscoveryRepositories(database)
	catalog, err := service.NewCatalogService(discovery.Jobs, discovery.Jobs, discovery.Companies,
		discovery.Locations, discovery.Filters)
	if err != nil {
		return nil, fmt.Errorf("initialize job catalog service: %w", err)
	}
	catalog.WithSources(repository.NewJobSourceStore(database))
	profileService := service.NewProfileService(database)
	autoApplyEngine := service.NewAutoApplyEngine(database).WithNATS(natsConn)
	enterpriseService := service.NewEnterpriseService(database)
	enterpriseHandler := controller.NewEnterpriseHandler(enterpriseService)

	verifier, err := middleware.NewJWTVerifier(cfg.JWTIssuer, cfg.JWTAudience,
		cfg.JWTVerificationKeys, authRepositories.Sessions)
	if err != nil {
		return nil, fmt.Errorf("initialize JWT verifier: %w", err)
	}
	handler, err := controller.NewHTTPHandler(controller.HTTPDependencies{
		Auth: auth, Applications: applications, Profile: profileService, AutoApply: autoApplyEngine,
		JWT: verifier, Memberships: authRepositories.Memberships,
		AuthConfig: controller.AuthHTTPConfig{
			SecureCookies: cfg.SecureCookies, FrontendRedirectURL: cfg.FrontendAuthCallback,
			IsProduction: cfg.Environment == config.EnvironmentProduction,
		},
		AllowedOrigins: cfg.AllowedOrigins,
		Readiness:      readinessChecker{database: database, nats: natsConn},
		Catalog:        catalog,
		Enterprise:     enterpriseHandler,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize HTTP API: %w", err)
	}
	return handler, nil
}

func buildOAuthProviders(ctx context.Context, cfg config.Config) (map[service.OAuthProvider]service.OAuthClient, error) {
	providers := make(map[service.OAuthProvider]service.OAuthClient, 2)
	if cfg.GoogleOAuth.Enabled() {
		client, err := service.NewGoogleOAuthClient(ctx, service.OAuthClientSettings{
			ClientID: cfg.GoogleOAuth.ClientID, ClientSecret: cfg.GoogleOAuth.ClientSecret,
			RedirectURL: cfg.GoogleOAuth.RedirectURL,
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("Google OIDC discovery failed: %w", err)
		}
		providers[service.OAuthProviderGoogle] = client
	}
	if cfg.GitHubOAuth.Enabled() {
		client, err := service.NewGitHubOAuthClient(service.OAuthClientSettings{
			ClientID: cfg.GitHubOAuth.ClientID, ClientSecret: cfg.GitHubOAuth.ClientSecret,
			RedirectURL: cfg.GitHubOAuth.RedirectURL,
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("initialize GitHub OAuth: %w", err)
		}
		providers[service.OAuthProviderGitHub] = client
	}
	return providers, nil
}

func buildWorkerRuntime(cfg config.Config, database *mongo.Database, taskService service.ApplicationTaskService,
	jetstreamClient jetstream.JetStream) (*worker.Runtime, error) {
	workerID, err := workerIdentity()
	if err != nil {
		return nil, err
	}
	runtimeConfig := worker.DefaultRuntimeConfig(workerID)
	runtimeConfig.ProvisionStreams = cfg.Environment != config.EnvironmentProduction
	runtimeConfig.ProvisionConsumers = cfg.Environment != config.EnvironmentProduction
	runtimeConfig.OutboxBatchSize = cfg.OutboxBatchSize
	runtimeConfig.OutboxConcurrency = min(cfg.WorkerConcurrency, cfg.OutboxBatchSize)
	runtimeConfig.SourceBatchSize = min(cfg.OutboxBatchSize, 500)
	runtimeConfig.SourceConcurrency = min(cfg.WorkerConcurrency, runtimeConfig.SourceBatchSize)
	runtimeConfig.ConsumerConcurrency = cfg.WorkerConcurrency
	runtimeConfig.ShutdownTimeout = cfg.ShutdownTimeout

	return worker.NewRuntime(runtimeConfig, worker.RuntimeDependencies{
		JetStream:        jetstreamClient,
		OutboxStore:      repository.NewOutboxStore(database),
		InboxStore:       repository.NewInboxStore(database),
		SourceSyncStore:  repository.NewJobSourceStore(database),
		SourceConnector:  job_sources.NewRegistry(nil),
		ApplicationTasks: taskService,
	}, func(err error) {
		slog.Error("background worker operation failed", "error", err)
	})
}

func runWorker(ctx context.Context, cfg config.Config, database *mongo.Database, taskService service.ApplicationTaskService) error {
	natsConn, jetstreamClient, err := connectJetStream(cfg)
	if err != nil {
		return err
	}
	defer natsConn.Close()
	runtime, err := buildWorkerRuntime(cfg, database, taskService, jetstreamClient)
	if err != nil {
		return fmt.Errorf("initialize worker runtime: %w", err)
	}
	slog.Info("OneRole worker started", "worker_id", workerLabel())
	return runtime.Run(ctx)
}

func connectJetStream(cfg config.Config) (*nats.Conn, jetstream.JetStream, error) {
	conn, err := nats.Connect(cfg.NATSURL,
		nats.Name("OneRole worker"), nats.Timeout(5*time.Second),
		nats.MaxReconnects(-1), nats.ReconnectWait(time.Second))
	if err != nil {
		return nil, nil, fmt.Errorf("connect to NATS: %w", err)
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("initialize JetStream client: %w", err)
	}
	return conn, js, nil
}

func serveHTTP(ctx context.Context, cfg config.Config, handler http.Handler) error {
	server := &http.Server{
		Addr: cfg.HTTPAddr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.RequestTimeout,
		WriteTimeout: cfg.RequestTimeout, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	serveErr := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	slog.Info("OneRole API started", "address", cfg.HTTPAddr)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return nil
	case err := <-serveErr:
		return fmt.Errorf("serve HTTP API: %w", err)
	}
}

type namedService struct {
	name string
	run  func(context.Context) error
}

func runServices(ctx context.Context, services ...namedService) error {
	groupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(services))
	for _, service := range services {
		service := service
		go func() { results <- result{name: service.name, err: service.run(groupCtx)} }()
	}
	select {
	case <-ctx.Done():
		cancel()
		for range services {
			<-results
		}
		return nil
	case result := <-results:
		cancel()
		for range services[1:] {
			<-results
		}
		if ctx.Err() != nil {
			return nil
		}
		if result.err != nil {
			return fmt.Errorf("%s service: %w", result.name, result.err)
		}
		return fmt.Errorf("%s service stopped unexpectedly", result.name)
	}
}

type readinessChecker struct {
	database *mongo.Database
	nats     *nats.Conn
}

func (checker readinessChecker) Check(ctx context.Context) error {
	if checker.database == nil {
		return errors.New("database is not configured")
	}
	if err := checker.database.Client().Ping(ctx, nil); err != nil {
		return errors.New("database is unavailable")
	}
	if checker.nats != nil && !checker.nats.IsConnected() {
		return errors.New("message broker is unavailable")
	}
	return nil
}

func workerIdentity() (string, error) {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "", errors.New("could not determine worker identity")
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid()), nil
}

func workerLabel() string {
	workerID, err := workerIdentity()
	if err != nil {
		return "unavailable"
	}
	return workerID
}
