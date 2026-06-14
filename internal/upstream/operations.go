package upstream

const MetadataTopStreamsOperation = `query TopStreams($first: Int!, $after: Cursor) { streams(first: $first, after: $after) { edges { cursor node { id title viewersCount previewImageURL(width: 440, height: 248) broadcaster { login displayName } game { name } } } } }`

const MetadataCategoriesOperation = `query TopCategories($first: Int!, $after: Cursor) { games(first: $first, after: $after) { edges { cursor node { id name viewersCount boxArtURL(width: 148, height: 198) } } } }`

const MetadataCategoryStreamsOperation = `query CategoryStreams($id: ID!, $first: Int!, $after: Cursor) { game(id: $id) { streams(first: $first, after: $after) { edges { cursor node { id title viewersCount previewImageURL(width: 440, height: 248) broadcaster { login displayName } game { name } } } } } }`

const MetadataSearchOperation = `query SearchResults($query: String!) { searchFor(userQuery: $query, platform: "web") { channels { items { id login displayName profileImageURL(width: 70) stream { id title viewersCount previewImageURL(width: 440, height: 248) game { name } } } } games { items { id name boxArtURL(width: 148, height: 198) } } } }`

const MetadataChannelOperation = `query ChannelByLogin($login: String!) { user(login: $login) { id login displayName description profileImageURL(width: 300) createdAt stream { id title viewersCount previewImageURL(width: 440, height: 248) createdAt game { name } } } }`

const MetadataChannelAboutOperation = `query ChannelAbout($login: String!) { user(login: $login) { id panels(hideExtensions: false) { id type ... on DefaultPanel { title description imageURL linkURL } } } }`

const PlaybackAccessTokenOperation = `query PlaybackAccessTokenLive($login: String!, $playerType: String!) { streamPlaybackAccessToken(channelName: $login, params: {platform: "web", playerBackend: "mediaplayer", playerType: $playerType}) { value signature __typename } }`

const VodPlaybackAccessTokenOperation = `query PlaybackAccessTokenVod($vodID: ID!, $playerType: String!) { videoPlaybackAccessToken(id: $vodID, params: {platform: "web", playerBackend: "mediaplayer", playerType: $playerType}) { value signature __typename } }`
