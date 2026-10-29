module github.com/bwmarrin/discordgo/examples/voice_receive

go 1.27.0

replace github.com/bwmarrin/discordgo => ../../

require (
	github.com/bwmarrin/discordgo v0.29.1-0.20260214123928-f43dd94faaac
	github.com/pion/rtp v1.10.6
	github.com/pion/webrtc/v4 v4.2.26
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)
