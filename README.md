# FlexModule
**Next Generation Module System for Chatanium**<br/>
We're completely stripping out Chatanium's existing Go Plugin-based module system and creating a new one here.

## Goals

![FlexModule Diagram](https://github.com/user-attachments/assets/1cf77e2c-ea04-47be-b2c5-1d4bb7bd9fc6)

Once our PoC implementation for multi-provider (Backend) is finalized, this codebase will be adapted to Chatanium v2.

## Design

* **Module system integration based on gRPC (w. protobuf)**<br/>
Ultimately, Chatanium's goal is to deploy/develop/operate better chatbots. Therefore, the existing module system has some limitations (https://github.com/thirdscam/chatanium/issues/10), and fixing them is our top priority. To solve these problems, we will implement a module system based on gRPC, which will provide cross-platform compilation and better security.

## Local Discord browser debugging

`.mcp.json` registers `discord-devtools` using Chrome DevTools MCP. It requires
Node.js LTS and npm, and attaches to an existing browser at
`http://127.0.0.1:9223`; it does not launch the browser or the bot runtime.

The local workstation has a separate `chatanium-discord-chromium` container:

```sh
docker start chatanium-discord-chromium
docker stop chatanium-discord-chromium
```

Its authenticated remote desktop is available at `https://localhost:3302`.
Accept the self-signed certificate only for the intended desktop address.
Desktop credentials are kept out of this repository. The browser opens
`https://discord.com/login`, where the account owner signs in manually.
The `chatanium-discord-chromium-profile` Docker volume retains the browser profile
across container stops. The existing Firefox container and its profile are separate.

The workstation's sandbox-preserving browser launcher is stored at
`~/.local/share/chatanium-discord-debug/chromium-params`. CDP is published only on
host loopback; do not expose it to the LAN or Internet, because it grants control
of the signed-in browser.

Use `list_network_requests` and `get_network_request` to inspect HTTP traffic.
MCP usage statistics and CrUX reporting are disabled. Sensitive network-header
redaction is enabled, but request/response bodies are not automatically redacted:
do not export login payloads, account tokens, or private messages.
