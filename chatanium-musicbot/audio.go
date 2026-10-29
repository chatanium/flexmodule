package main

import (
	"bufio"
	"io"
	Url "net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/jonas747/dca"
	Provider "github.com/thirdscam/chatanium-flexmodule/chatanium-musicbot/provider"
	DiscordModule "github.com/thirdscam/chatanium-flexmodule/shared/discord-v1/module"
)

var MUSIC_PATH = filepath.Join(os.TempDir(), "chatanium-musicbot")

// DownloadMusic downloads the music from the given URL and returns the path to the downloaded file.
func DownloadMusic(rawURL string, musicId Provider.MusicID) error {
	if isExistMusic(musicId) {
		return nil
	}

	makeDirectory()

	_, err := Url.ParseRequestURI(rawURL)
	if err != nil {
		log.Error("Failed to parse URL", "error", err)
		return err
	}

	file, err := os.Create(getMusicPath(musicId))
	if err != nil {
		log.Error("Failed to create file", "error", err)
		return err
	}

	ok, err := download(rawURL, file)
	if err != nil {
		log.Error("Failed to download file", "error", err)
		return err
	}

	<-ok
	return nil
}

func RemoveMusic(musicId Provider.MusicID) {
	err := os.Remove(getMusicPath(musicId))
	if err != nil {
		log.Error("Failed to remove file", "error", err)
	}
}

func PlayMusic(vc *DiscordModule.VoiceClient, musicId Provider.MusicID, pause chan bool, skip chan bool) {
	file, err := os.Open(getMusicPath(musicId))
	if err != nil {
		log.Error("Failed to open file", "error", err)
		return
	}
	defer file.Close()

	decoder := dca.NewDecoder(file)
	isPaused := false
	frameCount := 0

playback:
	for {
		select {
		case <-pause:
			isPaused = !isPaused
			if isPaused {
				log.Debug("Music paused")
			} else {
				log.Debug("Music resumed")
			}
			continue

		case <-skip:
			log.Debug("Playback skipped")
			break playback

		default:
			if isPaused {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			frame, err := decoder.OpusFrame()
			if err == io.EOF {
				log.Debug("Playback finished")
				break playback
			}
			if err != nil {
				log.Error("Failed to read opus frame", "error", err)
				break playback
			}

			vc.Send(frame, 5, 5000)
			frameCount++

			time.Sleep(20 * time.Millisecond)

			if frameCount%500 == 0 {
				log.Debug("Sending audio frames", "count", frameCount)
			}
		}
	}

	log.Debug("Playback ended", "total_frames", frameCount)
}

func makeDirectory() error {
	if _, err := os.Stat(MUSIC_PATH); os.IsNotExist(err) {
		err := os.MkdirAll(MUSIC_PATH, 0o755)
		if err != nil {
			log.Error("Failed to create directory", "error", err)
			return err
		}
	}
	return nil
}

func getMusicPath(musicId Provider.MusicID) string {
	return filepath.Join(MUSIC_PATH, string(musicId))
}

func isExistMusic(musicId Provider.MusicID) bool {
	_, err := os.Stat(getMusicPath(musicId))
	if os.IsNotExist(err) {
		return false
	} else if err != nil {
		log.Debug("File not found", "error", err)
	}
	return true
}

func download(rawURL string, file *os.File) (chan bool, error) {
	log.Debug("Downloading", "url", rawURL)

	encodeSession, err := dca.EncodeFile(rawURL, dca.StdEncodeOptions)
	if err != nil {
		log.Error("Failed to encode file", "error", err)
		return nil, err
	}

	reader := bufio.NewReader(encodeSession)
	writer := bufio.NewWriter(file)

	isWriting := make(chan bool)
	go func() {
		chunkCnt := 0
		for {
			buf := make([]byte, 4096)
			n, err := reader.Read(buf)
			if err != nil {
				file.Close()
				encodeSession.Cleanup()
				if err == io.EOF {
					return
				}
				log.Debug("ffmpeg Read Error", "error", err)
				return
			}

			_, err = writer.Write(buf[:n])
			if err != nil {
				log.Debug("ffmpeg Write Error", "error", err)
				return
			}

			err = writer.Flush()
			if err != nil {
				log.Debug("ffmpeg Flush Error", "error", err)
				return
			}

			if chunkCnt == 5 {
				log.Debug("Ready to play")
				isWriting <- true
			}

			chunkCnt++
		}
	}()

	return isWriting, nil
}
