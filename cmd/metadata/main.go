package main

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"streamclone/internal/config"
	"streamclone/internal/httpx"
	"streamclone/internal/log"
	"streamclone/internal/metadata/api"
	"streamclone/internal/metadata/cache"
	"streamclone/internal/metadata/follow"
	"streamclone/internal/metadata/gql"
	"streamclone/internal/metadata/helix"
	"streamclone/internal/metrics"
)

func main() {
	cfg, err := config.Load()
	logger := log.New("metadata", cfg.LogLevel)
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}

	opts, err := goredis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("redis url parse failed", "err", err)
		os.Exit(1)
	}
	rdb := goredis.NewClient(opts)

	provider := gql.NewStaticProvider(cfg.Upstream.TwitchClientID, cfg.Upstream.UserAgent)
	gqlClient := gql.New(cfg.Upstream, provider)
	helixClient := helix.New(
		cfg.TwitchAPIURL,
		cfg.TwitchTokenURL,
		cfg.TwitchOAuthClientID,
		cfg.TwitchOAuthClientSecret,
		cfg.Upstream.UserAgent,
	)

	store := cache.NewRedisStore(rdb)
	c := cache.New(store, cfg.MetaCacheTTL, cfg.StaleTTL)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("postgres connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	h := api.New(c, gqlClient).
		WithFollowStore(follow.NewStore(pool)).
		WithHelix(helixClient).
		WithExternalSources(cfg.TwitchTrackerAPIURL, cfg.RedditAPIURL, cfg.Upstream.UserAgent).
		WithRedditOptions(api.RedditOptions{
			Provider:       cfg.RedditProvider,
			BaseURL:        cfg.RedditAPIURL,
			OAuthAPIURL:    cfg.RedditOAuthAPIURL,
			TokenURL:       cfg.RedditTokenURL,
			ClientID:       cfg.RedditClientID,
			ClientSecret:   cfg.RedditClientSecret,
			AccessToken:    cfg.RedditAccessToken,
			HTMLFallback:   cfg.RedditHTMLFallback,
			ThirdPartyURL:  cfg.RedditThirdPartyURL,
			ThirdPartyKey:  cfg.RedditThirdPartyKey,
			ScraperURL:     cfg.ScraperAPIURL,
			ScraperKey:     cfg.ScraperAPIKey,
			LSFLowPriority: cfg.RedditLSFLowPriority,
		}).
		WithYouTubeOptions(api.YouTubeOptions{
			Provider: cfg.YouTubeProvider,
			APIKey:   cfg.YouTubeAPIKey,
			APIURL:   cfg.YouTubeAPIBaseURL,
		}).
		WithSetupWelcome(api.SetupWelcomeOptions{
			Profile:               cfg.StreamcloneProfile,
			DevTokenImportEnabled: cfg.TwitchDevTokenImport,
			OAuthClientID:         cfg.TwitchOAuthClientID,
			OAuthClientSecret:     cfg.TwitchOAuthClientSecret,
			ClipperServiceURL:     cfg.ClipperServiceURL,
		})

	srv := httpx.New("metadata", cfg.HTTPAddr, logger, metrics.HTTPMiddleware("metadata"), httpx.CORS, httpx.NewRateLimiter(20, 40).Middleware)
	srv.AddReady(func(ctx context.Context) error {
		if err := rdb.Ping(ctx).Err(); err != nil {
			return err
		}
		return pool.Ping(ctx)
	})
	h.Mount(srv.Router)

	if err := srv.Run(context.Background()); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
