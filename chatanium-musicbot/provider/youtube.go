package Provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/lrstanley/go-ytdlp"
	"github.com/thirdscam/chatanium-flexmodule/chatanium-musicbot/util"
)

var log hclog.Logger

func init() {
	log = hclog.New(&hclog.LoggerOptions{
		Name:       "MusicBot/Provider",
		Level:      hclog.LevelFromString("DEBUG"),
		JSONFormat: true,
		Output:     os.Stderr,
	})
}

type Youtube struct{}

func (y *Youtube) Start() {
	ytdlp.MustInstall(context.Background(), nil)
	log.Info("yt-dlp installed, starting...")

	beforeVersion, err := exec.Command(util.GetYtdlpPath(), "--version", "--quiet", "--no-warnings").Output()
	if err != nil {
		log.Error("Failed to get yt-dlp version", "error", err)
	}

	err = exec.Command(util.GetYtdlpPath(), "-U", "--quiet", "--no-warnings").Run()
	if err != nil {
		log.Error("Failed to update yt-dlp", "error", err)
	}

	afterVersion, err := exec.Command(util.GetYtdlpPath(), "--version", "--quiet", "--no-warnings").Output()
	if err != nil {
		log.Error("Failed to get yt-dlp version", "error", err)
	}

	if string(beforeVersion) == string(afterVersion) {
		log.Info("yt-dlp is already up to date", "version", strings.TrimSuffix(string(beforeVersion), "\n"))
	} else {
		log.Info("yt-dlp updated", "from", strings.TrimSuffix(string(beforeVersion), "\n"), "to", strings.TrimSuffix(string(afterVersion), "\n"))
	}

	go func() {
		for {
			time.Sleep(12 * time.Hour)
			err = exec.Command(util.GetYtdlpPath(), "-U", "--quiet", "--no-warnings").Run()
			if err != nil {
				log.Error("Failed to update yt-dlp", "error", err)
			}

			afterVersion, err = exec.Command(util.GetYtdlpPath(), "--version", "--quiet", "--no-warnings").Output()
			if err != nil {
				log.Error("Failed to get yt-dlp version", "error", err)
			}

			if string(beforeVersion) != string(afterVersion) {
				log.Info("yt-dlp auto-updated", "from", strings.TrimSuffix(string(beforeVersion), "\n"), "to", strings.TrimSuffix(string(afterVersion), "\n"))
				beforeVersion = afterVersion
			} else {
				log.Debug("yt-dlp is already up to date", "version", strings.TrimSuffix(string(afterVersion), "\n"))
			}
		}
	}()
}

func (y *Youtube) GetMusic(query string) ([]Music, error) {
	if util.IsYoutubeUrl(query) {
		log.Debug("Query is a URL", "url", query)
		return getUrl(query)
	}

	log.Debug("Query is a search", "query", query)
	return getSearch(query)
}

func getSearch(query string) ([]Music, error) {
	cmd := exec.Command(util.GetYtdlpPath(), fmt.Sprintf("ytsearch:'%s'", query), "--quiet", "--no-warnings", "--skip-download", "--format=bestaudio/best", "-O", "id,title,url,thumbnail")
	r, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	result := strings.Split(string(r), "\n")
	if len(result) < 4 {
		return nil, fmt.Errorf("no results found or invalid query (len() < 4)")
	}

	isValidUrl := util.IsUrl(result[2]) && util.IsUrl(result[3])
	if !isValidUrl {
		return nil, fmt.Errorf("invalid query response (invalid url)")
	}

	return []Music{
		{
			Id:           MusicID("YT:" + util.GetSha256Hash(result[0])),
			Title:        result[1],
			RawUrl:       result[2],
			ThumbnailUrl: result[3],
			Type:         "youtube",
		},
	}, nil
}

func getUrl(url string) ([]Music, error) {
	cmd := exec.Command(util.GetYtdlpPath(), url, "--quiet", "--no-warnings", "--skip-download", "--format=bestaudio/best", "-O", "id,title,url,thumbnail")
	r, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	execResult := strings.Split(strings.TrimSuffix(string(r), "\n"), "\n")
	result := []Music{}

	if len(execResult) < 4 {
		return nil, fmt.Errorf("no results found or invalid query (len() < 4)")
	}

	for i := 0; i < len(execResult); i += 4 {
		if execResult[i] == "" {
			break
		}

		isValidUrl := util.IsUrl(execResult[i+2]) && util.IsUrl(execResult[i+3])
		if !isValidUrl {
			return nil, fmt.Errorf("invalid query response (invalid url)")
		}

		if i+3 > len(execResult) {
			log.Debug("Failed to parse result partially: result length is not a multiple of 4")
			break
		}

		result = append(result, Music{
			Id:           MusicID("YT:" + util.GetSha256Hash(execResult[i])),
			Title:        execResult[i+1],
			RawUrl:       execResult[i+2],
			ThumbnailUrl: execResult[i+3],
			Type:         "youtube",
		})
	}

	if len(result) == 0 {
		log.Debug("Cannot find result")
	}

	return result, nil
}
