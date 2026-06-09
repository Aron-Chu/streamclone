package main

import (
	"context"
	"os"

	"github.com/redis/go-redis/v9"

	chatauth "streamclone/internal/chat/auth"
	"streamclone/internal/chat/batch"
	"streamclone/internal/chat/enrich"
	"streamclone/internal/chat/hub"
	"streamclone/internal/chat/ircconn"
	"streamclone/internal/chat/parse"
	"streamclone/internal/chat/pubsub"
	"streamclone/internal/config"
	"streamclone/internal/httpx"
	"streamclone/internal/log"
	"streamclone/internal/metrics"
)

func main() {
	cfg, err := config.Load()
	logger := log.New("chat", cfg.LogLevel)
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}

	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("redis url parse failed", "err", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(opt)

	ps := pubsub.New(rdb)

	enricher := enrich.New(rdb, cfg.DeltaDebounceMS, logger)

	batcher := batch.New(cfg.BatchWindowMS, func(channel string, data []byte) {
		ps.Publish(context.Background(), channel, data)
	})

	var mgr *ircconn.Manager

	handler := func(line string) {
		msg, ok := parse.ParseLine(line)
		if !ok {
			return
		}
		metrics.ChatMessagesIn.Inc()
		batcher.Add(msg.Channel, batch.BatchMessage{
			ID:        msg.ID,
			User:      msg.User,
			Color:     msg.Color,
			Badges:    msg.Badges,
			TS:        msg.TS,
			Fragments: enricher.Tokenize(msg.Channel, msg.Text),
		})
	}

	mgr = ircconn.NewManager(cfg.Upstream.TwitchIRCURL, cfg.MaxChannelsPerSocket, handler, logger)

	authHandler := chatauth.New(chatauth.NewRedisStore(rdb), chatauth.Config{
		ClientID:              cfg.TwitchOAuthClientID,
		ClientSecret:          cfg.TwitchOAuthClientSecret,
		RedirectURL:           cfg.TwitchOAuthRedirectURL,
		FrontendURL:           cfg.FrontendOrigin,
		AuthURL:               cfg.TwitchOAuthURL,
		TokenURL:              cfg.TwitchTokenURL,
		ValidateURL:           cfg.TwitchValidateURL,
		APIURL:                cfg.TwitchAPIURL,
		Scopes:                cfg.TwitchAuthScopes,
		DevTokenImportEnabled: cfg.TwitchDevTokenImport,
		CookieSecret:          cfg.AuthCookieSecret,
		CookieSameSite:        cfg.AuthCookieSameSite,
	}, logger)
	sender := ircconn.NewSenderManager(cfg.Upstream.TwitchIRCURL, authHandler, logger)
	h := hub.New(mgr, ps, cfg.ClientSendQueue, 500, logger).WithAuth(authHandler, sender)

	srv := httpx.New("chat", cfg.HTTPAddr, logger, metrics.HTTPMiddleware("chat"), httpx.CORSForOrigin(cfg.FrontendOrigin), httpx.NewRateLimiter(20, 40).Middleware)
	srv.AddReady(func(ctx context.Context) error {
		return rdb.Ping(ctx).Err()
	})
	authHandler.Routes(srv.Router)
	srv.Router.Get("/v1/ws", h.ServeHTTP)

	if err := srv.Run(context.Background()); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
