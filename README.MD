# SummerRateChecker 🏖️

A Go-powered Discord bot that monitors Summer.fi vault interest rates and sends alerts when rates change significantly.

## Features

- 📊 Real-time monitoring of Summer.fi/Morpho vault interest rates
- 🔔 Discord alerts when rates change beyond your threshold
- 🏷️ Nickname support for easy vault identification
- ⚙️ Configurable thresholds per vault
- 🤖 Discord bot commands for easy management
- 📈 Rich Discord embeds with rate change visualization
- 🔄 Automatic market pair detection from Summer.fi URLs
- 🕒 Customizable check intervals
- 📝 Support for quoted nicknames with spaces

## Requirements

- Go 1.20 or newer
- A Discord bot token and webhook URL

## Configuration

Copy `config.toml.example` to `config.toml` and fill in your details, or use environment variables as described in the file.

## Quick Start

### 1. Install Go

Download and install Go from [golang.org](https://golang.org/dl/)

### 2. Set up Discord Bot

1. Go to [Discord Developer Portal](https://discord.com/developers/applications)
2. Create a new application
3. Go to "Bot" section and create a bot
4. Copy the bot token
5. Under "OAuth2" → "URL Generator":
   - Select "bot" scope
   - Select "Send Messages", "Embed Links" permissions
   - Invite bot to your server

### 3. Set up Discord Webhook (Optional)

1. In your Discord server, go to Server Settings → Integrations
2. Create a webhook for the channel where you want alerts
3. Copy the webhook URL

### 4. Configure the Bot

**Configuration File**
Copy `config.toml.example` to `config.toml` and fill in values:
```toml
[discord]
token = "your_bot_token"
guild_id = "123456789012345678"
webhook_url = "https://discord.com/api/webhooks/..."

[monitor]
check_interval_minutes = 60
```

### 5. Build and Run

```bash
# Clone the repository
git clone <your-repo>
cd SummerRateChecker

# Run the build script
chmod +x build.sh
./build.sh

# Run the bot
./bin/SummerRateChecker
```

Or run directly with Go:
```bash
go run main.go
```

## Discord Commands

All commands start with `!`:

### Vault Management
- `!enroll <summer.fi_url> <"nickname"> <threshold> [channel]`
  - Example: `!enroll https://pro.summer.fi/ethereum/morphoblue/borrow/WBTC-USDC/1234#overview "My WBTC Vault" 0.5 #rate-alerts`
  - Nicknames can contain spaces and must be enclosed in quotes
  - Threshold is in percentage points (0.5 = alert on ±0.5% change)
  - The channel is optional; if omitted, alerts go to the current channel

- `!unenroll <vault_id>`
  - Remove a vault from monitoring

- `!list`
  - Show all enrolled vaults with their market pairs, thresholds, and alert channels (shows 'unknown' if unset)

### Monitoring
- `!status`
  - Show current rates for all vaults

- `!check`
  - Force an immediate rate check

- `!threshold <vault_id> <new_threshold>`
  - Update the alert threshold for a vault

- `!interval`
  - Show current check interval

- `!help`
  - Show help message

## Finding Vault Information

To enroll a vault, you need to provide the full Summer.fi URL:

1. Go to your vault on Summer.fi
2. Copy the URL from your browser
3. The URL format is: `https://pro.summer.fi/ethereum/morphoblue/borrow/MARKET-PAIR/VAULT-ID#overview`
   - Example: `https://pro.summer.fi/ethereum/morphoblue/borrow/WBTC-USDC/1234#overview`
   - The bot will automatically extract both the vault ID and market pair from the URL

## Example Usage

```
# Enroll a vault using Summer.fi URL
!enroll https://pro.summer.fi/ethereum/morphoblue/borrow/WBTC-USDC/1234#overview "My WBTC Vault" 0.5 #wbtc-alerts

# Check current status
!status

# List all vaults
!list

# Update threshold
!threshold 1234 0.8
```

## Alert Format

When rates change, you'll get rich Discord embeds like:

```
📈 Rate Alert: My WBTC Vault

Current Rate: 5.80%
Previous Rate: 5.20%
Change: increased by 0.60%

2 minutes ago
```

## Project Structure

```
.
├── main.go                 # Application entry point
├── internal/
│   ├── bot/               # Discord bot commands
│   ├── config/            # Configuration management
│   ├── monitor/           # Rate monitoring logic
│   ├── morpho/            # Morpho API client
│   ├── storage/           # Data storage (in-memory and file)
│   └── types/             # Shared types
├── config.toml.example    # Configuration template
└── build.sh              # Build script
```

## Configuration

### Finding Discord Guild ID

1. Enable Developer Mode in Discord (User Settings → Advanced → Developer Mode)
2. Right-click your server name in Discord
3. Click "Copy Server ID"

## Development

### Building from Source

```bash
git clone <repository>
cd SummerRateChecker
go mod tidy
go build -o bin/SummerRateChecker main.go
```

### Running in Development

```bash
go run main.go
```

### Dependencies

- `discordgo` - Discord API client
- `viper` - Configuration management
- `zap` - Structured logging
- `graphql` - GraphQL client for Morpho API

## Troubleshooting

### Bot Not Responding
- Verify Discord token is correct
- Check bot has message permissions in channels
- Ensure guild ID matches your Discord server

### No Alerts
- Verify webhook URL is configured
- Check vault IDs are correct
- Ensure rate thresholds aren't too high

### Build Issues
- Make sure Go 1.21+ is installed
- Run `go mod tidy` to fetch dependencies
- Check for any missing environment variables

## Extending the Bot

The codebase is designed to be easily extensible:

- Add new storage backends (replace in-memory storage)
- Support additional protocols beyond Morpho
- Add more sophisticated alert logic
- Implement a web dashboard
- Add Telegram notifications

## License

MIT [License](LICENSE)

---

Built with ❤️ using Go 🦫 and DiscordGo 🤖