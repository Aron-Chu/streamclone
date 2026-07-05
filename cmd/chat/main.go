package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/redis/go-redis/v9"

	chatauth "streamclone/internal/chat/auth"
	"streamclone/internal/chat/batch"
	"streamclone/internal/chat/enrich"
	"streamclone/internal/chat/hub"
	"streamclone/internal/chat/ingest"
	"streamclone/internal/chat/ircconn"
	"streamclone/internal/chat/parse"
	"streamclone/internal/chat/pubsub"
	"streamclone/internal/config"
	"streamclone/internal/emote/render"
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
	if cfg.EmoteRenderOnChatObserved {
		observePub := render.NewObservePublisher(rdb, logger)
		enricher.SetEmoteObserver(func(channel string, frag batch.Fragment) {
			observePub.TryPublish(render.ObservePayload{
				EmoteID:      frag.ID,
				Provider:     frag.Provider,
				ChannelLogin: channel,
				Scale:        "1x",
			})
		})
	}

	batcher := batch.New(cfg.BatchWindowMS, func(channel string, data []byte) {
		ps.Publish(context.Background(), channel, data)
	})
	eventBatcher := batch.NewEventBatcher(cfg.BatchWindowMS, func(channel string, data []byte) {
		ps.Publish(context.Background(), channel, data)
	})
	archiver := ingest.New(cfg.AnalyticsServiceURL, cfg.ChatLogPersistEnabled)

	var mgr *ircconn.Manager

	handler := func(line string) {
		if msg, ok := parse.ParseLine(line); ok {
			metrics.ChatMessagesIn.Inc()
			frags := enricher.Tokenize(msg.Channel, msg.Text, msg.Emotes)
			batcher.Add(msg.Channel, batch.BatchMessage{
				ID:        msg.ID,
				User:      msg.User,
				Login:     msg.Login,
				Color:     msg.Color,
				Badges:    msg.Badges,
				TS:        msg.TS,
				Fragments: frags,
			})
			if archiver.Enabled() {
				fragsJSON, _ := json.Marshal(frags)
				archiver.ForwardMessages(ingest.Message{
					Channel:     msg.Channel,
					Login:       msg.Login,
					DisplayName: msg.User,
					MessageID:   msg.ID,
					Text:        msg.Text,
					Fragments:   fragsJSON,
					TS:          msg.TS,
				})
			}
			return
		}
		if ev, ok := parse.ParseEvent(line); ok {
			summary := ev.SummaryText()
			eventBatcher.Add(ev.Channel, batch.ChatEvent{
				Kind:        ev.Kind,
				TargetLogin: ev.TargetLogin,
				ActorLogin:  ev.ActorLogin,
				DurationSec: ev.DurationSec,
				Reason:      ev.Reason,
				MessageID:   ev.MessageID,
				TextPreview: ev.TextPreview,
				NoticeMsgID: ev.NoticeMsgID,
				DisplayText: ev.DisplayText,
				TS:          ev.TS,
				SummaryText: summary,
			})
			if archiver.Enabled() {
				archiver.ForwardEvents(ingest.Event{
					Kind:        ev.Kind,
					Channel:     ev.Channel,
					ActorLogin:  ev.ActorLogin,
					TargetLogin: ev.TargetLogin,
					DurationSec: ev.DurationSec,
					Reason:      ev.Reason,
					MessageID:   ev.MessageID,
					TextPreview: firstNonEmpty(ev.TextPreview, ev.DisplayText, summary),
					TS:          ev.TS,
				})
			}
		}
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
		ClipperAuthSyncPath:   cfg.ClipperAuthSyncPath,
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
