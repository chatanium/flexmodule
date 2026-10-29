package main

import (
	"context"
	"io"
	Url "net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	Provider "github.com/thirdscam/chatanium-flexmodule/chatanium-musicbot/provider"
	DiscordModule "github.com/thirdscam/chatanium-flexmodule/shared/discord-v1/module"
	"github.com/thirdscam/chatanium-flexmodule/shared/media/opus"
)

var MUSIC_PATH = filepath.Join(os.TempDir(), "chatanium-musicbot")
var mediaContext, cancelMedia = context.WithCancel(context.Background())
var downloads sync.Map // Provider.MusicID -> *musicDownload

type musicDownload struct {
	ready chan struct{}
	done  chan struct{}
	err   error // Published by closing done.
}

// DownloadMusic returns when enough encoded audio is buffered to start playback.
// The bounded streaming encoder continues until EOF or module shutdown.
func DownloadMusic(rawURL string, musicId Provider.MusicID) error {
	if existing, ok := downloads.Load(musicId); ok {
		return waitDownload(existing.(*musicDownload))
	}
	if isExistMusic(musicId) {
		return nil
	}
	if err := makeDirectory(); err != nil {
		return err
	}
	if _, err := Url.ParseRequestURI(rawURL); err != nil {
		return err
	}
	download := &musicDownload{ready: make(chan struct{}), done: make(chan struct{})}
	actual, loaded := downloads.LoadOrStore(musicId, download)
	if loaded {
		return waitDownload(actual.(*musicDownload))
	}
	file, err := os.Create(getMusicPath(musicId))
	if err != nil {
		download.err = err
		close(download.done)
		close(download.ready)
		downloads.Delete(musicId)
		return err
	}
	go func() {
		writer := &readyWriter{file: file, ready: download.ready}
		err := opus.Encode(mediaContext, rawURL, writer, 64)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		download.err = err
		if err != nil {
			log.Error("Failed to encode music", "error", err)
			os.Remove(getMusicPath(musicId))
			downloads.Delete(musicId)
		}
		close(download.done)
		if !writer.signaled {
			close(download.ready)
		}
	}()
	return waitDownload(download)
}

func waitDownload(download *musicDownload) error {
	select {
	case <-mediaContext.Done():
		return mediaContext.Err()
	case <-download.ready:
		select {
		case <-download.done:
			return download.err
		default:
			return nil
		}
	}
}

type readyWriter struct {
	file     *os.File
	ready    chan struct{}
	bytes    int
	signaled bool
}

func (w *readyWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	w.bytes += n
	if !w.signaled && w.bytes >= 5*4096 {
		w.signaled = true
		close(w.ready)
	}
	return n, err
}

func RemoveMusic(musicId Provider.MusicID) {
	downloads.Delete(musicId)
	if err := os.Remove(getMusicPath(musicId)); err != nil {
		log.Error("Failed to remove file", "error", err)
	}
}

// growingMusicReader waits for the encoder instead of treating a temporary file
// EOF (including a partially written frame) as the end of the song.
type growingMusicReader struct {
	file     *os.File
	download *musicDownload
	skip     <-chan bool
}

func (r *growingMusicReader) Read(p []byte) (int, error) {
	for {
		if err := mediaContext.Err(); err != nil {
			return 0, err
		}
		n, err := r.file.Read(p)
		if n > 0 || err != io.EOF || r.download == nil {
			return n, err
		}
		select {
		case <-r.download.done:
			// Retry after completion; the final write may have raced the EOF.
			n, err = r.file.Read(p)
			if n == 0 && err == io.EOF && r.download.err != nil {
				return 0, r.download.err
			}
			return n, err
		case <-mediaContext.Done():
			return 0, mediaContext.Err()
		case <-r.skip:
			return 0, context.Canceled
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func PlayMusic(vc *DiscordModule.VoiceClient, musicId Provider.MusicID, pause chan bool, skip chan bool) {
	file, err := os.Open(getMusicPath(musicId))
	if err != nil {
		log.Error("Failed to open file", "error", err)
		return
	}
	defer file.Close()
	reader := &growingMusicReader{file: file, skip: skip}
	if download, ok := downloads.Load(musicId); ok {
		reader.download = download.(*musicDownload)
	}
	decoder := opus.NewDecoder(reader)
	isPaused := false
	frameCount := 0
	for {
		select {
		case <-mediaContext.Done():
			return
		case <-pause:
			isPaused = !isPaused
			continue
		case <-skip:
			log.Debug("Playback skipped")
			return
		default:
		}
		if isPaused {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		frame, err := decoder.OpusFrame(mediaContext)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Error("Failed to read opus frame", "error", err)
			break
		}
		vc.Send(frame, 5, 5000)
		frameCount++
		time.Sleep(20 * time.Millisecond)
		if frameCount%500 == 0 {
			log.Debug("Sending audio frames", "count", frameCount)
		}
	}
	log.Debug("Playback ended", "total_frames", frameCount)
}

func makeDirectory() error {
	return os.MkdirAll(MUSIC_PATH, 0o755)
}

func getMusicPath(musicId Provider.MusicID) string {
	return filepath.Join(MUSIC_PATH, string(musicId))
}

func isExistMusic(musicId Provider.MusicID) bool {
	_, err := os.Stat(getMusicPath(musicId))
	return err == nil
}
