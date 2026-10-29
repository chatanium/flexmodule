package Provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/lrstanley/go-ytdlp"
	"github.com/thirdscam/chatanium-flexmodule/chatanium-musicbot/util"
)

type Youtube struct{}

// Start checks the image-provided executable. Modules never install or update
// media binaries: yt-dlp and its JS runtime/ejs closure are image dependencies.
func (y *Youtube) Start() error {
	_, err := youtubeExecutable()
	return err
}

func youtubeExecutable() (string, error) {
	path, err := exec.LookPath("yt-dlp")
	if err != nil {
		return "", fmt.Errorf("yt-dlp executable is required in the pinned module media image PATH (including a supported YouTube JS runtime and yt-dlp-ejs): %w", err)
	}
	return path, nil
}

func (y *Youtube) GetMusic(ctx context.Context, query string) ([]Music, error) {
	if !util.IsYoutubeUrl(query) {
		query = "ytsearch:" + query
	}
	executable, err := youtubeExecutable()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// BuildCommand is the v1.5.6 API. Own the pipes instead of Run's unbounded
	// accumulated stdout/log results. No binary resolver/install API is invoked.
	cmd := ytdlp.New().SetExecutable(executable).BuildCommand(ctx,
		"--ignore-config", "--no-update", "--quiet", "--no-warnings",
		"--skip-download", "--format=bestaudio/best",
		"--print", "%(id)j", "--print", "%(title)j", "--print", "%(url)j", "--print", "%(thumbnail)j",
		"--", query)
	// Give yt-dlp its normal interrupt path to tear down extractor subprocesses;
	// BuildCommand's positive WaitDelay still bounds escalation to Kill.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		stdout.Close()
		return nil, fmt.Errorf("start image-provided yt-dlp: %w", err)
	}
	limited := &io.LimitedReader{R: stdout, N: 16 << 20}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	result := []Music{}
	var fields [4]string
	index := 0
	var parseErr error
	for scanner.Scan() {
		if err := json.Unmarshal(scanner.Bytes(), &fields[index]); err != nil {
			parseErr = fmt.Errorf("invalid yt-dlp metadata field: %w", err)
			break
		}
		index++
		if index != len(fields) {
			continue
		}
		if fields[0] == "" || !util.IsUrl(fields[2]) || !util.IsUrl(fields[3]) {
			parseErr = fmt.Errorf("invalid yt-dlp metadata response (missing ID or URL)")
			break
		}
		result = append(result, Music{
			Id:    MusicID("YT:" + util.GetSha256Hash(fields[0])),
			Title: fields[1], RawUrl: fields[2], ThumbnailUrl: fields[3], Type: "youtube",
		})
		index = 0
	}
	if parseErr == nil {
		parseErr = scanner.Err()
	}
	if parseErr == nil && limited.N == 0 {
		parseErr = fmt.Errorf("yt-dlp metadata exceeds 16MiB; request a smaller playlist")
	}
	if parseErr != nil {
		cancel()
	}
	stdout.Close()
	waitErr := cmd.Wait()
	if err := ctx.Err(); err != nil && parseErr == nil {
		parseErr = err
	}
	if parseErr != nil {
		return nil, parseErr
	}
	if waitErr != nil {
		return nil, fmt.Errorf("yt-dlp metadata extraction failed; check the pinned media image's JS runtime/ejs closure and source availability: %w", waitErr)
	}
	if index != 0 || len(result) == 0 {
		return nil, fmt.Errorf("no results found or incomplete yt-dlp metadata for %s", strings.TrimSpace(query))
	}
	return result, nil
}
