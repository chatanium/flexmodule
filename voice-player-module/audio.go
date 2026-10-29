package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	DiscordModule "github.com/thirdscam/chatanium-flexmodule/shared/discord-v1/module"
	"github.com/thirdscam/chatanium-flexmodule/shared/media/opus"
)

var mediaContext, cancelMedia = context.WithCancel(context.Background())

// convertToDCA writes raw length-prefixed Opus frames using image-provided ffmpeg.
func convertToDCA(ctx context.Context, inputPath string, outputPath string) error {
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	err = opus.Encode(ctx, inputPath, outFile, 128)
	if closeErr := outFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(outputPath)
		return fmt.Errorf("failed to encode Opus frames: %w", err)
	}
	log.Info("Successfully converted to DCA", "input", inputPath, "output", outputPath)
	return nil
}

// playDCA plays a DCA file through the existing module voice client.
func playDCA(ctx context.Context, vc *DiscordModule.VoiceClient, dcaPath string) error {
	file, err := os.Open(dcaPath)
	if err != nil {
		return fmt.Errorf("failed to open DCA file: %w", err)
	}
	defer file.Close()
	decoder := opus.NewDecoder(file)
	frameCount := 0
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		frame, err := decoder.OpusFrame(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read opus frame: %w", err)
		}
		vc.Send(frame, 5, 5000)
		frameCount++
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if frameCount%100 == 0 {
			log.Debug("Sending audio frames", "count", frameCount)
		}
	}
	log.Info("Finished playing audio", "total_frames", frameCount)
	return nil
}
