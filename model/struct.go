package model

// Config is the struct of the loaded config file
type Config struct {
	Input struct {
		Aria2 struct {
			Aria2Server string `json:"aria2-server"`
			Aria2Key    string `json:"aria2-key"`
		} `json:"aria2"`
	} `json:"input"`
	Output struct {
		Telegram struct {
			BotKey string `json:"bot-key"`
			UserID string `json:"user-id"`
		} `json:"telegram"`
	} `json:"output"`
	MaxIndex int `json:"max-index"`
	Language string `json:"language"`

	// DownloadFolder is the folder aria2 downloads into
	DownloadFolder string `json:"downloadFolder"`
	// Library is the base path of the organized media library
	Library OrganizeConfig `json:"organize"`

	Log struct {
		LogPath string `json:"logPath"`
		ErrPath string `json:"errPath"`
		Level   string `json:"level"`
	} `json:"log"`
}

// OrganizeConfig holds the media library layout used by the organizer
type OrganizeConfig struct {
	// Enabled turns post-download organizing on/off
	Enabled bool `json:"enabled"`
	// AniList enables AniList lookups for anime confirmation
	AniList bool `json:"anilist"`
	// DeleteArchive deletes the original archive after successful extraction
	DeleteArchive bool `json:"deleteArchive"`
	Movies        string `json:"movies"`
	Series        string `json:"series"`
	Anime         string `json:"anime"`
	Music         string `json:"music"`
	Documents     string `json:"documents"`
	Archives      string `json:"archives"`
	Others        string `json:"others"`
	// YouTube is the base path for yt-dlp YouTube downloads
	YouTube string `json:"youtube"`
	// Services is the base path for yt-dlp downloads from other services
	Services string `json:"services"`
	// YtdlpPath is the path to the yt-dlp binary
	YtdlpPath string `json:"ytdlpPath"`
	// YtdlpQuality is the yt-dlp format selector (default: 1080p + best audio)
	YtdlpQuality string `json:"ytdlpQuality"`
	// YtdlpCookies is the path to a cookies.txt file for yt-dlp
	YtdlpCookies string `json:"ytdlpCookies"`
	// YtdlpProxy is a proxy URL for yt-dlp
	YtdlpProxy string `json:"ytdlpProxy"`
	// YtdlpEmbed embeds metadata + thumbnail (needs ffmpeg)
	YtdlpEmbed bool `json:"ytdlpEmbed"`
}
