package main

import (
	"context"
	"os"

	goredis "github.com/redis/go-redis/v9"

	"streamclone/internal/config"
	"streamclone/internal/httpx"
	"streamclone/internal/log"
	"streamclone/internal/metadata/api"
	"streamclone/internal/metadata/cache"
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

	h := api.New(c, gqlClient).
		WithHelix(helixClient).
		WithExternalSources(cfg.TwitchTrackerAPIURL, cfg.RedditAPIURL, cfg.Upstream.UserAgent).
		WithRedditOptions(api.RedditOptions{
			Provider:      cfg.RedditProvider,
			BaseURL:       cfg.RedditAPIURL,
			OAuthAPIURL:   cfg.RedditOAuthAPIURL,
			TokenURL:      cfg.RedditTokenURL,
			ClientID:      cfg.RedditClientID,
			ClientSecret:  cfg.RedditClientSecret,
			AccessToken:   cfg.RedditAccessToken,
			HTMLFallback:  cfg.RedditHTMLFallback,
			ThirdPartyURL: cfg.RedditThirdPartyURL,
			ThirdPartyKey: cfg.RedditThirdPartyKey,
			FirecrawlURL:  cfg.FirecrawlAPIURL,
			FirecrawlKey:  cfg.FirecrawlAPIKey,
		})

	srv := httpx.New("metadata", cfg.HTTPAddr, logger, metrics.HTTPMiddleware("metadata"), httpx.CORS, httpx.NewRateLimiter(20, 40).Middleware)
	srv.AddReady(func(ctx context.Context) error {
		return rdb.Ping(ctx).Err()
	})
	h.Mount(srv.Router)

	if err := srv.Run(context.Background()); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
