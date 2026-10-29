package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	Url "net/url"
	"sync"
)

func Str2Int64(s string) (int64, error) {
	n := new(big.Int)
	n, ok := n.SetString(s, 10)
	if !ok {
		return -1, fmt.Errorf("failed to convert ID: %v", s)
	}
	return n.Int64(), nil
}

func GetSha256Hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func IsUrl(url string) bool {
	_, err := Url.ParseRequestURI(url)
	return err == nil
}

func IsYoutubeUrl(url string) bool {
	u, err := Url.ParseRequestURI(url)
	if err != nil {
		return false
	}

	return u.Host == "www.youtube.com" || u.Host == "youtube.com" || u.Host == "youtu.be"
}

func IsYoutubePlaylist(url string) bool {
	u, err := Url.ParseRequestURI(url)
	if err != nil {
		return false
	}

	return u.Host == "www.youtube.com" && u.Path == "/playlist"
}

func WithLock(mu *sync.Mutex, fn func()) {
	mu.Lock()
	defer mu.Unlock()
	fn()
}

func WithRLock(mu *sync.RWMutex, fn func()) {
	mu.RLock()
	defer mu.RUnlock()
	fn()
}
