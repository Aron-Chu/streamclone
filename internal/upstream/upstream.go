package upstream

type Endpoints struct {
	TwitchGQLURL   string `env:"TWITCH_GQL_URL" envDefault:"https://gql.twitch.tv/gql"`
	TwitchClientID string `env:"TWITCH_CLIENT_ID" envDefault:"kimne78kx3ncx6brgo4mv6wki5h1ko"`
	TwitchUsherURL string `env:"TWITCH_USHER_URL" envDefault:"https://usher.ttvnw.net"`
	TwitchIRCURL   string `env:"TWITCH_IRC_URL" envDefault:"wss://irc-ws.chat.twitch.tv:443"`
	SevenTVAPIURL  string `env:"SEVENTV_API_URL" envDefault:"https://7tv.io/v3"`
	SevenTVCDNURL  string `env:"SEVENTV_CDN_URL" envDefault:"https://cdn.7tv.app"`
	FFZAPIURL      string `env:"FFZ_API_URL" envDefault:"https://api.frankerfacez.com/v1"`
	UserAgent      string `env:"TWITCH_USER_AGENT" envDefault:"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"`
}
