module github.com/thirdscam/chatanium-flexmodule/chatanium-musicbot

go 1.24.0

replace github.com/thirdscam/chatanium-flexmodule => ../

replace github.com/bwmarrin/discordgo => ../discordgo

require (
	github.com/bwmarrin/discordgo v0.28.1
	github.com/hashicorp/go-hclog v1.6.3
	github.com/hashicorp/go-plugin v1.6.3
	github.com/jonas747/dca v0.0.0-20210930103944-155f5e5f0cc7
	github.com/lrstanley/go-ytdlp v0.0.0-20250219030852-4f99aecdc40c
	github.com/thirdscam/chatanium-flexmodule v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.71.0
)

require (
	github.com/ProtonMail/go-crypto v1.1.5 // indirect
	github.com/cloudflare/circl v1.6.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/hashicorp/yamux v0.1.1 // indirect
	github.com/jonas747/ogg v0.0.0-20161220051205-b4f6f4cf3757 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/oklog/run v1.0.0 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f // indirect
	google.golang.org/protobuf v1.36.5 // indirect
)
