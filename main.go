package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/thirdscam/chatanium-flexmodule/shared"
	"github.com/thirdscam/chatanium-flexmodule/shared/core-v1"
	"github.com/thirdscam/chatanium-flexmodule/shared/discord-v1"
	"github.com/thirdscam/chatanium-flexmodule/shared/discord-v1/convert/struct2buf"
	discordRuntime "github.com/thirdscam/chatanium-flexmodule/shared/discord-v1/runtime"

	_ "github.com/joho/godotenv/autoload"
)

var (
	dgSession *discordgo.Session
	log       hclog.Logger
)

var (
	GUILD_ID      string
	DISCORD_TOKEN string
	PLUGIN_PATH   string
)

func loadEnv() {
	GUILD_ID = os.Getenv("GUILD_ID")
	if GUILD_ID == "" {
		log.Error("GUILD_ID is not set")
		os.Exit(1)
	}

	DISCORD_TOKEN = os.Getenv("DISCORD_TOKEN")
	if DISCORD_TOKEN == "" {
		log.Error("DISCORD_TOKEN is not set")
		os.Exit(1)
	}

	PLUGIN_PATH = os.Getenv("PLUGIN_PATH")
	if PLUGIN_PATH == "" {
		log.Error("PLUGIN_PATH is not set")
		os.Exit(1)
	}
}

func main() {
	log = hclog.New(&hclog.LoggerOptions{
		Name:                 "Runtime",
		Level:                hclog.LevelFromString("DEBUG"),
		Color:                hclog.AutoColor,
		ColorHeaderAndFields: true,
	})

	loadEnv()

	// We don't want to see the plugin logs.
	// log.SetOutput(io.Discard)

	log.Debug("starting up", "path", PLUGIN_PATH)

	session, err := discordgo.New("Bot " + DISCORD_TOKEN)
	if err != nil {
		log.Error("Error creating Discord session", "error", err.Error())
		os.Exit(1)
	}
	dgSession = session

	dgSession.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Info("Logged in", "username", s.State.User.Username, "discriminator", s.State.User.Discriminator)
	})

	dgSession.Open()

	struct2buf.DiscordSession = session

	// Create Discord helper
	discordHelper := discordRuntime.NewDiscordHelper(dgSession)

	// Create Voice helper
	voiceHelper := discordRuntime.NewVoiceHelper(dgSession, log)

	// Create runtime plugin map with Discord helper and voice helper
	runtimePluginMap := shared.CreateRuntimePluginMap(discordHelper, voiceHelper)

	// We're a host. Start by launching the plugin process.
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: shared.Handshake,
		Plugins:         runtimePluginMap,
		Cmd:             exec.Command(PLUGIN_PATH),
		Logger:          log.ResetNamed("Module").Named("TestModule"),
		AllowedProtocols: []plugin.Protocol{
			plugin.ProtocolGRPC,
		},
	})
	defer client.Kill()

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		log.Error("Error creating gRPC Client", "error", err.Error())
		os.Exit(1)
	}

	RunCoreV1(rpcClient)

	// After Core is run, we can run another provider
	// This blocks until Ctrl+C is pressed
	RunDiscordV1(rpcClient, discordHelper)

	dgSession.Close()
}

func RunCoreV1(client plugin.ClientProtocol) {
	// Request the plugin
	raw, err := client.Dispense("core-v1")
	if err != nil {
		log.Error("Core", "error", err.Error())
		os.Exit(1)
	}

	// Getting the plugin symbol
	hook, ok := raw.(core.Hook)
	if !ok {
		log.Error("Core", "error", "Plugin has no 'core-v1' plugin symbol")
		os.Exit(1)
	}

	manifest, err := hook.GetManifest()
	if err != nil {
		log.Error("Core", "error", err.Error())
		os.Exit(1)
	}
	log.Debug("Core", "manifest", hclog.Fmt("%+v", manifest))

	status, err := hook.GetStatus()
	if err != nil {
		log.Error("Core", "error", err.Error())
		os.Exit(1)
	}
	log.Debug("Core", "status", hclog.Fmt("%+v", status))

	hook.OnStage("MODULE_INIT")
	log.Debug("Core", "stage", "MODULE_INIT")

	status, err = hook.GetStatus()
	if err != nil {
		log.Error("Core", "error", err.Error())
		os.Exit(1)
	}
	log.Debug("Core", "status", hclog.Fmt("%+v", status))
}

func RunDiscordV1(client plugin.ClientProtocol, discordHelper discord.Helper) {
	start := time.Now()

	// Request the plugin
	raw, err := client.Dispense("discord-v1")
	if err != nil {
		log.Error("Discord", "error", err.Error())
		os.Exit(1)
	}

	// Getting the plugin symbol (RuntimeClients)
	runtimeClients, ok := raw.(discord.RuntimeClients)
	if !ok {
		log.Error("Discord", "error", "Plugin has no 'discord-v1' plugin symbol")
		os.Exit(1)
	}

	// Get hook client to call module functions
	hook := runtimeClients.GetHook()

	// Use the runtime's Discord helper, not the module's helper client
	resp := hook.OnInit(discordHelper)
	log.Debug("Discord", "initresp", hclog.Fmt("%+v", resp))
	if len(resp.Interactions) != 0 {
		log.Debug("Discord", "AppID", dgSession.State.User.ID)
		for i, c := range resp.Interactions {
			log.Debug("["+strconv.Itoa(i+1)+"/"+strconv.Itoa(len(resp.Interactions))+"] INTERACTION", "BEFORE", hclog.Fmt("%+v", c))
			// int64 to int64 pointer
			c.DefaultMemberPermissions = nil
			_i, err := dgSession.ApplicationCommandCreate(dgSession.State.User.ID, GUILD_ID, c)
			if err != nil {
				log.Error("Discord", "error", err.Error())
				os.Exit(1)
			}
			log.Debug("["+strconv.Itoa(i+1)+"/"+strconv.Itoa(len(resp.Interactions))+"] INTERACTION", "AFTER", hclog.Fmt("%+v", _i))
		}
	}

	dgSession.AddHandler(func(s *discordgo.Session, i *discordgo.MessageCreate) {
		log.Debug("Discord", "type", "MESSAGE_CREATE", "message", hclog.Fmt("%+v", i.Message))
		log.Info(">> RECV::MESSAGE", "ID", "G:"+i.GuildID+"|C:"+i.ChannelID, "USER", "U:"+i.Author.ID+" ("+i.Author.Username+")", "CONTENT", i.Message.Content)
		hook.OnCreateChatMessage(i.Message)
	})

	dgSession.AddHandler(func(s *discordgo.Session, i *discordgo.MessageDelete) {
		log.Debug("Discord", "type", "MESSAGE_DELETE", "message", hclog.Fmt("%+v", i.Message))
		log.Info(">> RECV::MESSAGE_DELETE", "ID", "G:"+i.GuildID+"|C:"+i.ChannelID, "MESSAGE_ID", i.ID)
		hook.OnCreateChatMessage(i.Message)
	})

	dgSession.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		log.Debug("Discord", "type", "INTERACTION_CREATE", "interaction", hclog.Fmt("%+v", i.Interaction))
		log.Info(">> RECV::INTERACTION", "ID", i.Interaction.ID, "TYPE", i.Type.String(), "NAME", i.ApplicationCommandData().Name, "USER", "U:"+i.Interaction.Member.User.ID+" ("+i.Interaction.Member.User.Username+")")
		hook.OnCreateInteraction(i.Interaction)
	})

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	log.Info("Ready to serve. (" + time.Since(start).Truncate(time.Millisecond).String() + ") (press Ctrl+C to exit)")
	<-stop
	fmt.Println()
	log.Info("Shutting down...")
}
